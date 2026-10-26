// Package web holds the ochq.uz landing page: the page template, its
// assets and the translations. internal/web serves them.
package web

import "embed"

//go:embed site i18n
var FS embed.FS
