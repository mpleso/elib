package elib

// Formats generic slices/arrays of structs as tables.

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

type row struct {
	cols []string
}

type col struct {
	name   string
	format string
	width  int
	maxLen int
	align
}

type table struct {
	cols []col
	rows []row
}

func (c *col) getWidth() int {
	if c.width != 0 {
		return c.width
	}
	return c.maxLen
}

func (c *col) displayName() string {
	// Map underscore to space so that title for "a_b" is "A B".
	return strings.Title(strings.Replace(c.name, "_", " ", -1))
}

type align int

const (
	alignCenter align = iota
	alignLeft
	alignRight
)

func formatCenteredString(s string, align align, width int) (v string) {
	l := len(s)
	nLeft, nRight := 0, 0
	if d := width - l; d > 0 {
		switch align {
		case alignCenter:
			nLeft = d / 2
			nRight = nLeft
			if d%2 != 0 {
				nLeft++
			}
		case alignLeft:
			nRight = d
		case alignRight:
			nLeft = d
		}
	}
	v = ""
	for i := 0; i < nLeft; i++ {
		v += " "
	}
	v += s
	for i := 0; i < nRight; i++ {
		v += " "
	}
	return
}

func (t *table) String() (s string) {
	s = ""
	for c := range t.cols {
		s += formatCenteredString(t.cols[c].displayName(), t.cols[c].align, t.cols[c].getWidth())
	}
	s += "\n"
	for r := range t.rows {
		for c := range t.rows[r].cols {
			s += formatCenteredString(t.rows[r].cols[c], t.cols[c].align, t.cols[c].getWidth())
		}
		s += "\n"
	}
	return
}

func Tabulate(x interface{}) (tab *table) {
	v := reflect.ValueOf(x)
	t := v.Type()
	if t.Kind() == reflect.Ptr {
		v = v.Elem()
		t = v.Type()
	}
	var (
		et      reflect.Type
		vLen    int
		isArray bool
	)
	switch t.Kind() {
	case reflect.Slice, reflect.Array:
		vLen = v.Len()
		et = t.Elem()
		isArray = true
	case reflect.Struct:
		vLen = 1
		et = t
		isArray = false
	default:
		panic("not slice or array")
	}

	tab = &table{}
	tab.cols = make([]col, et.NumField())
	tab.rows = make([]row, vLen)
	for c := range tab.cols {
		f := et.Field(c)
		if w := f.Tag.Get("width"); len(w) > 0 {
			if x, err := strconv.ParseUint(w, 10, 0); err != nil {
				panic(fmt.Errorf("bad width for field %s: %s", f.Name, err))
			} else {
				tab.cols[c].width = int(x)
			}
		}
		if w := f.Tag.Get("format"); len(w) > 0 {
			tab.cols[c].format = w
		}
		tab.cols[c].name = f.Name
		tab.cols[c].maxLen = len(tab.cols[c].name)
		if c > 0 {
			tab.cols[c].maxLen += 1
		}
	}

	for r := 0; r < vLen; r++ {
		f := v
		if isArray {
			f = f.Index(r)
		}
		for c := range tab.cols {
			fc := f.Field(c)
			var v string
			if tab.cols[c].format != "" {
				v = fmt.Sprintf(tab.cols[c].format, fc)
			} else {
				v = fmt.Sprintf("%v", fc)
			}
			tab.rows[r].cols = append(tab.rows[r].cols, v)
			if l := len(v); l > tab.cols[c].maxLen {
				tab.cols[c].maxLen = l
			}
		}
	}

	return tab
}
