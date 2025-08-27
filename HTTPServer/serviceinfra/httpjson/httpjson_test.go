package httpjson

import (
	"fmt"
	"reflect"
	"testing"
)

func TestLiteralNull(t *testing.T) {
	pn1, pn2 := LiteralNullFor[*int](), LiteralNullFor[*int]()
	fmt.Println(IsLiteralNull(pn1)) // true
	fmt.Println(IsLiteralNull(pn2)) // true
	fmt.Println(IsLiteralNull(nil)) // false
	x := 5
	fmt.Println(IsLiteralNull(&x)) // false

	sl1, sl2 := LiteralNull(reflect.TypeFor[[]bool]()).([]bool), []bool{}
	fmt.Println(IsLiteralNull(sl1))      // true
	fmt.Println(IsLiteralNull(sl2))      // false
	fmt.Println(IsLiteralNull([]bool{})) // false
	sl := []string{"a"}
	fmt.Println(IsLiteralNull(sl)) // false

	m1, m2 := LiteralNullFor[map[string]int](), (map[string]int)(nil)
	fmt.Println(IsLiteralNull(m1))  // true
	fmt.Println(IsLiteralNull(m2))  // false
	fmt.Println(IsLiteralNull(nil)) // false
	m := map[string]int{"a": 1}
	fmt.Println(IsLiteralNull(m)) // false
}
