package changelog

import "embed"

//go:embed entries/*.md assets/*
var FS embed.FS
