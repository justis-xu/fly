// Package assets 内置静态资源（geo 数据等），运行期零外部下载。
package assets

import _ "embed"

//go:embed geoip.metadb
var GeoMetadb []byte

//go:embed GeoSite.dat
var GeoSiteDat []byte

//go:embed ASN.mmdb
var ASNMMDB []byte
