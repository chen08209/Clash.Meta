package adapter

type UrlTestCheck func(url string, name string, delay uint16)

var UrlTestHook UrlTestCheck
