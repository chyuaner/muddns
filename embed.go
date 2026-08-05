package main

import (
	"embed"
)

//go:embed web/templates/*
var templateFS embed.FS
