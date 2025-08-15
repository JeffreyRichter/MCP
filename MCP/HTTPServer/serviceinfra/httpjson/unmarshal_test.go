package httpjson

import (
	"encoding/json/v2"
	"testing"
	"time"
)

type MyJson struct {
	Flag    *bool      `name:"flag"`
	Integer *int       `name:"integer" minval:"1" maxval:"6"`
	Float   *float32   `name:"float" minval:"1.5" maxval:"6.5"`
	String  *string    `name:"string" minlen:"3" maxlen:"64" regx:"^[a-zA-Z0-9_]+$"`
	Enum    *string    `name:"enum" enums:"red,green,blue"`
	Colors  []string   `name:"colors" minlen:"1" maxlen:"10" enums:"red,green,blue"`
	Date    *time.Time `name:"date"`
	Contact *struct {
		Phone *string `name:"phone" minlen:"10" maxlen:"14" regx:"^(\\+\\d{1,2}\\s)?\\(?\\d{3}\\)?[\\s.-]\\d{3}[\\s.-]\\d{4}$"`
		Email *string `name:"email" minlen:"6" maxlen:"64" regx:"^([a-zA-Z0-9._%-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,})$"`
	} `name:"contact"`
}

func TestJson2Struct(t *testing.T) {
	j := `{
		"colors": null,
		"flag": null,
		"integer": 1,
		"float": 2.3,
		"string": "four",
		"enum": "blue",
		"date": "2019-10-12T07:20:50.52Z",
		"contact": {
			"phone": "425-123-4567",
			"email": "abc@def.com"
		}
	}`
	/* [
		"red",
		"green",
		"blue"
	]*/

	var jsonObj map[string]any // any is one of: nil, bool, float64, string, []any (array), map[string]any
	err := json.Unmarshal([]byte(j), &jsonObj)
	print(err, jsonObj)

	/*s, err := obj[MyJson](jsonObj)
	fmt.Println(httpjson.IsLiteralNull(s.Colors))
	fmt.Printf("err=%v\n\ns=%v\n\n", err, s)
	if err != nil {
		t.Error(err)
	}*/
}
