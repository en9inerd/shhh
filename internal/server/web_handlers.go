package server

import "github.com/en9inerd/shhh/internal/validator"

const (
	baseTmpl  = "base"
	mainTmpl  = "main"
	errorTmpl = "error"

	msgKey     = "message"
	pinKey     = "pin"
	expKey     = "exp"
	expUnitKey = "expUnit"
	keyKey     = "key"
)

type createMsgForm struct {
	Message string
	Exp     int
	MaxExp  string
	ExpUnit string
	validator.Validator
}

type showMsgForm struct {
	Key     string
	Message string
	validator.Validator
}

type templateData struct {
	Form          any
	PinSize       int
	CurrentYear   int
	Theme         string
	Branding      string
	URL           string // canonical URL for the page
	BaseURL       string // base URL for the site (protocol://domain)
	PageTitle     string // SEO-optimized page title
	PageDesc      string // page description for meta tags
	IsMessagePage bool   // true for message pages (should not be indexed)
}
