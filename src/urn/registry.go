package urn

import (
	"reflect"
	"strings"
)

type Registry struct {
	typeToHandler   map[string]Handler
	domainToHandler map[string]Handler
}

var registry = &Registry{
	typeToHandler:   make(map[string]Handler),
	domainToHandler: make(map[string]Handler),
}

func RegisterHandler[T any](handler Handler) {
	t := reflect.TypeFor[T]()

	registry.typeToHandler[t.Name()] = handler
	registry.domainToHandler[handler.Domain()] = handler
}
func RegisterHandlerUntyped(handler Handler) {
	typeName := strings.ToUpper(handler.Domain()[0:1]) + handler.Domain()[1:]

	registry.typeToHandler[typeName] = handler
	registry.domainToHandler[handler.Domain()] = handler
}

func RegisterTypedHandler[T any](options ...func(Handler) Handler) {
	_, ok := GetHandlerByType[T]()
	if !ok {
		opts := append(
			[]func(Handler) Handler{
				WithDomain(reflect.TypeFor[T]().Name()),
			}, options...,
		)
		RegisterHandler[T](
			NewHandlerWithOptions(opts...),
		)
	}
}

func GetHandlerByType[T any]() (Handler, bool) {
	t := reflect.TypeFor[T]()

	handler, ok := registry.typeToHandler[t.Name()]
	return handler, ok
}

func GetHandlerByDomain(domain string) (Handler, bool) {
	handler, ok := registry.domainToHandler[domain]
	return handler, ok
}
