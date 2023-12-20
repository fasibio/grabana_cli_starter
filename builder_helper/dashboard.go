package builderhelper

import "fmt"

type DashboardConstant string

func (d DashboardConstant) AsVarQuote() string {
	return fmt.Sprintf("\"%s\"", d.String())
}

func (d DashboardConstant) String() string {
	return string(d)
}

type DashboardVariable string

func (d DashboardVariable) AsVar() string {
	return fmt.Sprintf("$%s", d)
}
func (d DashboardVariable) AsVarQuote() string {
	return fmt.Sprintf("\"%s\"", d.AsVar())
}

func (d DashboardVariable) String() string {
	return string(d)
}

type Pixel int

func (p Pixel) String() string {
	return fmt.Sprintf("%dpx", p)
}

func Pointer[T any](v T) *T {
	return &v
}

func AsQuoted[T any](v T) string {
	return fmt.Sprintf("\"%v\"", v)
}
