// Package messages uses MyGo's ICU catalog directly, without a replacement
// localization engine. MyGo generate registers packaged languages through
// its generated catalog_gen.go; this direct formatter also works in unit tests.
package messages

import (
	"embed"
	"github.com/egoist/mygo/i18n"
)

//go:embed en.json zh-Hans.json
var lamMessageFiles embed.FS
var lamCatalog = func() *i18n.Catalog {
	c, e := i18n.NewCatalog(lamMessageFiles, "en")
	if e != nil {
		panic(e)
	}
	return c
}()

func Text(locale, key string) string { return lamCatalog.Format(locale, key, nil) }
