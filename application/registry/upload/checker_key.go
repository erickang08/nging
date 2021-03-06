/*
   Nging is a toolbox for webmasters
   Copyright (C) 2018-present  Wenhui Shen <swh@admpub.com>

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package upload

import (
	"fmt"
	"net/url"
	"time"

	"github.com/webx-top/com"
	"github.com/webx-top/echo"
	"github.com/webx-top/echo/middleware/tplfunc"
	"github.com/webx-top/echo/subdomains"
)

// APIKey API Key
type APIKey interface {
	APIKey() string
}

// Token 生成签名
func Token(values ...interface{}) string {
	var urlValues url.Values
	if len(values) == 1 {
		switch t := values[0].(type) {
		case url.Values:
			urlValues = t
		case map[string][]string:
			urlValues = url.Values(t)
		default:
			urlValues = tplfunc.URLValues(values...)
		}
	} else {
		urlValues = tplfunc.URLValues(values...)
	}
	urlValues.Del(`token`)
	var apiKey string
	if cfg, ok := echo.Get(`DefaultConfig`).(APIKey); ok {
		apiKey = cfg.APIKey()
	}
	return com.SafeBase64Encode(com.Token(apiKey, com.Str2bytes(urlValues.Encode())))
}

// URLParam URLParam(`news`,`refid`,123)
func URLParam(subdir string, values ...interface{}) string {
	var urlValues url.Values
	if len(values) == 1 {
		switch t := values[0].(type) {
		case url.Values:
			urlValues = t
		case map[string][]string:
			urlValues = url.Values(t)
		default:
			urlValues = tplfunc.URLValues(values...)
		}
	} else {
		urlValues = tplfunc.URLValues(values...)
	}
	if len(urlValues.Get(`refid`)) == 0 {
		urlValues.Set(`refid`, `0`)
	}
	if SetURLParamDefaultValue != nil {
		SetURLParamDefaultValue(&urlValues)
	}
	unixtime := fmt.Sprint(time.Now().Unix())
	urlValues.Set(`time`, unixtime)
	urlValues.Del(`token`)
	urlValues.Set(`token`, Token(urlValues))
	return subdir + `?` + urlValues.Encode()
}

var (
	//BackendUploadPath 后台上传网址路径
	BackendUploadPath = `/manager/upload`
	//FrontendUploadPath 前台上传网址路径
	FrontendUploadPath = `/user/upload`
	//SetURLParamDefaultValue 设置参数默认值
	SetURLParamDefaultValue func(*url.Values)
)

// BackendUploadURL 构建后台上传网址
func BackendUploadURL(subdir string, values ...interface{}) string {
	return BackendURL() + BackendUploadPath + `/` + URLParam(subdir, values...)
}

// FrontendUploadURL 构建前台上传网址
func FrontendUploadURL(subdir string, values ...interface{}) string {
	return FrontendURL() + FrontendUploadPath + `/` + URLParam(subdir, values...)
}

// BackendURL 后台网址
func BackendURL() string {
	prefix, _ := echo.Get(`BackendPrefix`).(string)
	return subdomains.Default.URL(prefix, `backend`)
}

// FrontendURL 前台网址
func FrontendURL() string {
	prefix, _ := echo.Get(`FrontendPrefix`).(string)
	return subdomains.Default.URL(prefix, `frontend`)
}
