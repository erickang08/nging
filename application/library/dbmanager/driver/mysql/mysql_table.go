/*

   Copyright 2016 Wenhui Shen <www.webx.top>

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

*/
package mysql

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// 获取数据表列表
func (m *mySQL) getTables() ([]string, error) {
	sqlStr := `SHOW TABLES`
	rows, err := m.newParam().SetCollection(sqlStr).Query()
	if err != nil {
		return nil, err
	}
	ret := []string{}
	for rows.Next() {
		var v sql.NullString
		err := rows.Scan(&v)
		if err != nil {
			return nil, err
		}
		ret = append(ret, v.String)
	}
	return ret, nil
}

func (m *mySQL) optimizeTables(tables []string, operation string) error {
	r := &Result{}
	defer m.AddResults(r)
	var op string
	switch operation {
	case `optimize`, `check`, `analyze`, `repair`:
		op = strings.ToUpper(operation)
	default:
		return errors.New(m.T(`不支持的操作: %s`, operation))
	}
	for _, table := range tables {
		table = quoteCol(table)
		r.SQL = op + ` TABLE ` + table
		r.Execs(m.newParam())
		if r.err != nil {
			return r.err
		}
	}
	r.end()
	return r.err
}

// tables： tables or views
func (m *mySQL) moveTables(tables []string, targetDb string) error {
	r := &Result{}
	r.SQL = `RENAME TABLE `
	targetDb = quoteCol(targetDb)
	for i, table := range tables {
		table = quoteCol(table)
		if i > 0 {
			r.SQL += `,`
		}
		r.SQL += table + " TO " + targetDb + "." + table
	}
	r.Exec(m.newParam())
	m.AddResults(r)
	return r.err
}

//删除表
func (m *mySQL) dropTables(tables []string, isView bool) error {
	r := &Result{}
	r.SQL = `DROP `
	if isView {
		r.SQL += `VIEW `
	} else {
		r.SQL += `TABLE `
	}
	for i, table := range tables {
		table = quoteCol(table)
		if i > 0 {
			r.SQL += `,`
		}
		r.SQL += table
	}
	r.Exec(m.newParam())
	m.AddResults(r)
	return r.err
}

//清空表
func (m *mySQL) truncateTables(tables []string) error {
	r := &Result{}
	defer m.AddResults(r)
	for _, table := range tables {
		table = quoteCol(table)
		r.SQL = `TRUNCATE TABLE ` + table
		r.Execs(m.newParam())
		if r.err != nil {
			return r.err
		}
	}
	r.end()
	return nil
}

type viewCreateInfo struct {
	View                 sql.NullString
	CreateView           sql.NullString
	Character_set_client sql.NullString
	Collation_connection sql.NullString
	Select               string
}

func (m *mySQL) tableView(name string) (*viewCreateInfo, error) {
	sqlStr := `SHOW CREATE VIEW ` + quoteCol(name)
	row, err := m.newParam().SetCollection(sqlStr).QueryRow()
	if err != nil {
		return nil, err
	}
	info := &viewCreateInfo{}
	err = row.Scan(&info.View, &info.CreateView, &info.Character_set_client, &info.Collation_connection)
	if err != nil {
		return info, err
	}
	info.Select = reView.ReplaceAllString(info.CreateView.String, ``)
	return info, nil
}

func (m *mySQL) copyTables(tables []string, targetDb string, isView bool) error {
	r := &Result{}
	r.SQL = `SET sql_mode = 'NO_AUTO_VALUE_ON_ZERO'`
	r.Execs(m.newParam())
	m.AddResults(r)
	if r.err != nil {
		return r.err
	}
	same := m.dbName == targetDb
	targetDb = quoteCol(targetDb)
	for _, table := range tables {
		var name string
		quotedTable := quoteCol(table)
		if same {
			name = `copy_` + table
			name = quoteCol(name)
		} else {
			name = targetDb + "." + quotedTable
		}
		if isView {
			r.SQL = `DROP VIEW IF EXISTS ` + name
			r.Execs(m.newParam())
			if r.err != nil {
				return r.err
			}

			viewInfo, err := m.tableView(table)
			if err != nil {
				return err
			}
			r.SQL = `CREATE VIEW ` + name + ` AS ` + viewInfo.Select
			r.Execs(m.newParam())
			if r.err != nil {
				return r.err
			}
			continue

		}
		r.SQL = `DROP TABLE IF EXISTS ` + name
		r.Execs(m.newParam())
		if r.err != nil {
			return r.err
		}

		r.SQL = `CREATE TABLE ` + name + ` LIKE ` + quotedTable
		r.Execs(m.newParam())
		if r.err != nil {
			return r.err
		}

		r.SQL = `INSERT INTO ` + name + ` SELECT * FROM ` + quotedTable
		r.Execs(m.newParam())
		if r.err != nil {
			return r.err
		}
	}
	r.end()
	return r.err
}

type fieldItem struct {
	Original     string
	ProcessField []string
	After        string
}

func (m *mySQL) alterTable(table string, newName string, fields []*fieldItem, foreign map[string]string, comment sql.NullString, engine string,
	collation string, autoIncrement sql.NullInt64, partitioning string) error {
	alter := []string{}
	create := len(table) == 0
	for _, field := range fields {
		alt := ``
		if len(field.ProcessField) > 0 {
			if !create {
				if len(field.Original) > 0 {
					alt += `CHANGE ` + quoteCol(field.Original)
				} else {
					alt += `ADD`
				}
			}
			alt += ` ` + strings.Join(field.ProcessField, ``)
			if !create {
				alt += ` ` + field.After
			}
		} else {
			alt = `DROP ` + quoteCol(field.Original)
		}
		alter = append(alter, alt)
	}
	for _, v := range foreign {
		alter = append(alter, v)
	}
	status := ``
	if comment.Valid {
		status += " COMMENT=" + quoteVal(comment.String)
	}
	if len(engine) > 0 {
		status += " ENGINE=" + quoteVal(engine)
	}
	if len(collation) > 0 {
		status += " COLLATE " + quoteVal(collation)
	}
	if autoIncrement.Valid {
		status += " AUTO_INCREMENT=" + fmt.Sprint(autoIncrement.Int64)
	}
	r := &Result{}
	if create {
		r.SQL = `CREATE TABLE ` + quoteCol(newName) + " (\n" + strings.Join(alter, ",\n") + "\n)" + status + partitioning
	} else {
		if table != newName {
			alter = append(alter, `RENAME TO `+quoteCol(newName))
		}
		if len(status) > 0 {
			status = strings.TrimLeft(status, ` `)
		}
		if len(alter) > 0 || len(partitioning) > 0 {
			r.SQL = `ALTER TABLE ` + quoteCol(table) + "\n" + strings.Join(alter, ",\n") + partitioning
		} else {
			return nil
		}
	}
	r.Exec(m.newParam())
	m.AddResults(r)
	return r.err
}

type indexItems struct {
	Type      string
	Name      string
	Columns   []string
	Operation string
}

func (m *mySQL) alterIndexes(table string, alter []*indexItems) error {
	alters := make([]string, len(alter))
	for k, v := range alter {
		if v.Operation == `DROP` {
			alters[k] = "\nDROP INDEX " + quoteCol(v.Name)
			continue
		}
		alters[k] = "\nADD " + v.Type + " "
		if v.Type == `PRIMARY` {
			alters[k] += "KEY "
		}
		if len(v.Name) > 0 {
			alters[k] += quoteCol(v.Name) + " "
		}
		alters[k] += "(" + strings.Join(v.Columns, ", ") + ")"
	}
	r := &Result{
		SQL: "ALTER TABLE " + quoteCol(table) + strings.Join(alters, ","),
	}
	r.Exec(m.newParam())
	m.AddResults(r)
	return r.err
}

func (m *mySQL) tableFields(table string) (map[string]*Field, []string, error) {
	sqlStr := `SHOW FULL COLUMNS FROM ` + quoteCol(table)
	rows, err := m.newParam().SetCollection(sqlStr).Query()
	if err != nil {
		return nil, nil, err
	}
	ret := map[string]*Field{}
	sorts := []string{}
	for rows.Next() {
		v := &FieldInfo{}
		err := rows.Scan(&v.Field, &v.Type, &v.Collation, &v.Null, &v.Key, &v.Default, &v.Extra, &v.Privileges, &v.Comment)
		if err != nil {
			return nil, nil, err
		}
		match := reField.FindStringSubmatch(v.Type.String)
		var defaultValue sql.NullString
		if v.Default.String != `` || reFieldDefault.MatchString(match[1]) {
			defaultValue.Valid = true
			defaultValue.String = v.Default.String
		}
		var onUpdate string
		omatch := reFieldOnUpdate.FindStringSubmatch(v.Extra.String)
		if len(omatch) > 1 {
			onUpdate = omatch[1]
		}
		privileges := map[string]int{}
		for k, v := range reFieldPrivilegeDelim.Split(v.Privileges.String, -1) {
			privileges[v] = k
		}
		sorts = append(sorts, v.Field.String)
		ret[v.Field.String] = &Field{
			Field:         v.Field.String,
			Full_type:     v.Type.String,
			Type:          match[1],
			Length:        match[2],
			Unsigned:      strings.TrimLeft(match[3]+match[4], ` `),
			Default:       defaultValue,
			Null:          v.Null.String == `YES`,
			AutoIncrement: sql.NullString{Valid: v.Extra.String == `auto_increment`},
			On_update:     onUpdate,
			Collation:     v.Collation.String,
			Privileges:    privileges,
			Comment:       v.Comment.String,
			Primary:       v.Key.String == "PRI",
		}
	}
	return ret, sorts, nil
}

func (m *mySQL) tableIndexes(table string) (map[string]*Indexes, []string, error) {
	sqlStr := `SHOW INDEX FROM ` + quoteCol(table)
	rows, err := m.newParam().SetCollection(sqlStr).Query()
	ret := map[string]*Indexes{}
	sorts := []string{}
	if err != nil {
		return ret, sorts, err
	}
	for rows.Next() {
		v := &IndexInfo{}
		err := rows.Scan(&v.Table, &v.Non_unique, &v.Key_name, &v.Seq_in_index,
			&v.Column_name, &v.Collation, &v.Cardinality, &v.Sub_part,
			&v.Packed, &v.Null, &v.Index_type, &v.Comment, &v.Index_comment)
		if err != nil {
			return ret, sorts, err
		}
		if _, ok := ret[v.Key_name.String]; !ok {
			ret[v.Key_name.String] = &Indexes{
				Columns: []string{},
				Lengths: []string{},
				Descs:   []string{},
			}
		}
		if v.Key_name.String == `PRIMARY` {
			ret[v.Key_name.String].Type = `PRIMARY`
		} else if v.Index_type.String == `FULLTEXT` {
			ret[v.Key_name.String].Type = `FULLTEXT`
		} else if v.Non_unique.Valid {
			ret[v.Key_name.String].Type = `INDEX`
		} else {
			ret[v.Key_name.String].Type = `UNIQUE`
		}
		ret[v.Key_name.String].Columns = append(ret[v.Key_name.String].Columns, v.Column_name.String)
		ret[v.Key_name.String].Lengths = append(ret[v.Key_name.String].Lengths, v.Sub_part.String)
		ret[v.Key_name.String].Descs = append(ret[v.Key_name.String].Descs, ``)

		sorts = append(sorts, v.Key_name.String)
	}
	return ret, sorts, nil
}

func (m *mySQL) tableForeignKeys(table string) (map[string]*ForeignKeyParam, []string, error) {
	sorts := []string{}
	result := map[string]*ForeignKeyParam{}
	sqlStr := `SHOW CREATE TABLE ` + quoteCol(table)
	row, err := m.newParam().SetCollection(sqlStr).QueryRow()
	ret := make([]sql.NullString, 2)
	if err != nil {
		return result, sorts, err
	}
	err = row.Scan(&ret[0], &ret[1])
	if err != nil {
		return result, sorts, err
	}
	matches := reForeignKey.FindAllStringSubmatch(ret[1].String, -1)
	for _, match := range matches {
		source := reQuotedCol.FindAllStringSubmatch(match[2], -1)
		target := reQuotedCol.FindAllStringSubmatch(match[5], -1)
		key := strings.Trim(match[1], "`")
		item := &ForeignKeyParam{
			Name:     key,
			Source:   source[0],
			Target:   target[0],
			OnDelete: `RESTRICT`,
			OnUpdate: `RESTRICT`,
		}
		for k, v := range item.Source {
			item.Source[k] = strings.Trim(v, "`")
		}
		for k, v := range item.Target {
			item.Target[k] = strings.Trim(v, "`")
		}
		if len(match[4]) > 0 {
			item.Database = strings.Trim(match[3], "`")
			item.Table = strings.Trim(match[4], "`")
		} else {
			item.Database = strings.Trim(match[4], "`")
			item.Table = strings.Trim(match[3], "`")
		}
		if len(match[6]) > 0 {
			item.OnDelete = match[6]
		}
		if len(match[7]) > 0 {
			item.OnUpdate = match[7]
		}
		result[key] = item
		sorts = append(sorts, key)
	}
	return result, sorts, nil
}

func (m *mySQL) referencablePrimary(tableName string) (map[string]*Field, []string, error) {
	r := map[string]*Field{}
	s, sorts, e := m.getTableStatus(m.dbName, tableName, true)
	if e != nil {
		return r, sorts, e
	}
	for tblName, table := range s {
		if tblName != tableName && table.FKSupport(m.getVersion()) {
			fields, _, err := m.tableFields(tblName)
			if err != nil {
				return r, sorts, err
			}
			for _, field := range fields {
				if field.Primary {
					if _, ok := r[tblName]; ok {
						delete(r, tblName)
						break
					}
					r[tblName] = field
				}
			}
		}
	}
	return r, sorts, nil
}

func (m *mySQL) processLength(length string) (string, error) {
	r := ``
	re, err := regexp.Compile("^\\s*\\(?\\s*" + EnumLength + "(?:\\s*,\\s*" + EnumLength + ")*\\s*\\)?\\s*$")
	if err != nil {
		return r, err
	}
	if re.MatchString(length) {
		re, err := regexp.Compile(EnumLength)
		if err != nil {
			return r, err
		}
		matches := re.FindAllStringSubmatch(length, -1)
		if len(matches) > 0 {
			r = "(" + strings.Join(matches[0], ",") + ")"
			return r, nil
		}
	}

	length = reFieldLengthInvalid.ReplaceAllString(length, ``)
	r = reFieldLengthNumber.ReplaceAllString(length, `($0)`)
	return r, nil
}

func (m *mySQL) processType(field *Field, collate string) (string, error) {
	r := ` ` + field.Type
	l, e := m.processLength(field.Length)
	if e != nil {
		return ``, e
	}
	r += l

	if reFieldTypeNumber.MatchString(field.Type) {
		for _, v := range UnsignedTags {
			if field.Unsigned == v {
				r += ` ` + field.Unsigned
			}
		}
	}

	if reFieldTypeText.MatchString(field.Type) {
		if len(field.Collation) > 0 {
			r += ` ` + collate + ` ` + quoteVal(field.Collation)
		}
	}
	return r, nil
}

func (m *mySQL) autoIncrement(oldTable string, autoIncrementCol string) (string, error) {
	autoIncrementIndex := " PRIMARY KEY"
	// don't overwrite primary key by auto_increment
	if len(oldTable) > 0 && len(autoIncrementCol) > 0 {
		indexes, sorts, err := m.tableIndexes(oldTable)
		if err != nil {
			return ``, err
		}
		_ = sorts
		orig := m.Form(`fields[` + autoIncrementCol + `][orig]`)
		for _, index := range indexes {
			exists := false
			for _, col := range index.Columns {
				if col == orig {
					exists = true
					break
				}
			}
			if exists {
				autoIncrementIndex = ""
				break
			}
			if index.Type == "PRIMARY" {
				autoIncrementIndex = " UNIQUE"
			}
		}
	}
	return " AUTO_INCREMENT" + autoIncrementIndex, nil
}

func (m *mySQL) processField(oldTable string, field *Field, typeField *Field, autoIncrementCol string) ([]string, error) {
	//com.Dump(field)
	r := []string{quoteCol(strings.TrimSpace(field.Field))}
	t, e := m.processType(field, "COLLATE")
	if e != nil {
		return r, e
	}
	r = append(r, t)
	if field.Null {
		r = append(r, ` NULL`)
	} else {
		r = append(r, ` NOT NULL`)
	}
	var defaultValue string
	if field.Default.Valid {
		var isRaw bool
		typeN := strings.ToLower(field.Type)
		switch typeN {
		case `bit`:
			if reFieldTypeBit.MatchString(field.Default.String) {
				isRaw = true
			}
		default:
			if !strings.Contains(typeN, `time`) {
				break
			}
			switch strings.ToUpper(field.Default.String) {
			case `CURRENT_TIMESTAMP`:
				isRaw = true
			case `CURRENT_TIME`, `CURRENT_DATE`:
				isRaw = strings.ToLower(m.DbAuth.Driver) == `sqlite`
			default:
				switch strings.ToLower(m.DbAuth.Driver) {
				case `pgsql`:
					isRaw = pgsqlFieldDefaultValue.MatchString(field.Default.String)
				}
			}
		}
		if isRaw {
			defaultValue = ` DEFAULT ` + field.Default.String
		} else {
			defaultValue = ` DEFAULT ` + quoteVal(field.Default.String)
		}
	}
	r = append(r, defaultValue)
	if field.AutoIncrement.Valid {
		v, e := m.autoIncrement(oldTable, autoIncrementCol)
		if e != nil {
			return r, e
		}
		r = append(r, v)
	}
	return r, nil
}

type ForeignKeyParam struct {
	Name     string
	Database string
	Table    string
	Source   []string
	Target   []string
	OnDelete string
	OnUpdate string
}

func (m *mySQL) formatForeignKey(foreignKey *ForeignKeyParam) (string, error) {
	source := make([]string, len(foreignKey.Source))
	for k, v := range foreignKey.Source {
		source[k] = quoteCol(v)
	}
	target := make([]string, len(foreignKey.Target))
	for k, v := range foreignKey.Target {
		target[k] = quoteCol(v)
	}
	r := " FOREIGN KEY (" + strings.Join(source, ", ") + ") REFERENCES " + quoteCol(foreignKey.Table)
	r += " (" + strings.Join(target, ", ") + ")" //! reuse $name - check in older MySQL versions
	re, err := regexp.Compile(`^(` + OnActions + `)\$`)
	if err != nil {
		return ``, err
	}
	if re.MatchString(foreignKey.OnDelete) {
		r += " ON DELETE " + foreignKey.OnDelete
	}
	if re.MatchString(foreignKey.OnUpdate) {
		r += " ON UPDATE " + foreignKey.OnUpdate
	}
	return r, nil
}

func (m *mySQL) tableTriggers(table string) (map[string]*Trigger, []string, error) {
	sqlStr := `SHOW TRIGGERS LIKE ` + quoteVal(table, '_', '%')
	r := map[string]*Trigger{}
	s := []string{}
	rows, err := m.newParam().SetCollection(sqlStr).Query()
	if err != nil {
		return r, s, err
	}
	for rows.Next() {
		v := &Trigger{}
		err = rows.Scan(&v.Trigger, &v.Event, &v.Table, &v.Statement, &v.Timing, &v.Created, &v.Sql_mode, &v.Definer, &v.Character_set_client, &v.Collation_connection, &v.Database_collation)
		if err != nil {
			return r, s, err
		}
		r[v.Trigger.String] = v
		s = append(s, v.Trigger.String)
	}
	return r, s, nil
}
