package tui

import "reflect"

func reflectString(v any) string { return reflect.ValueOf(v).String() }
