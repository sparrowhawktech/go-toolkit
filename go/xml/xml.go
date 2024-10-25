package xml

import (
	"bytes"
	"encoding/xml"
	"io"
	"strconv"
	"strings"
	"time"

	"sparrowhawktech/toolkit/util"
)

type Element struct {
	XMLName    xml.Name    `xml:",name"`
	Attributes []*xml.Attr `xml:",any,attr"`
	Text       string      `xml:",chardata"`
	Children   []*Element  `xml:",any"`
	spaceMap   map[string]string
}

func (o *Element) AddElement(space *string, name string, text string) *Element {
	child := Element{
		XMLName:    xml.Name{Local: name},
		Attributes: nil,
		Text:       text,
		Children:   nil,
	}
	if space != nil {
		child.XMLName.Space = *space
	}
	if o.Children == nil {
		o.Children = make([]*Element, 0)
	}
	o.Children = append(o.Children, &child)
	return &child
}

func (o *Element) AddAttribute(space *string, name string, value string) *xml.Attr {
	attr := xml.Attr{
		Name:  xml.Name{Local: name},
		Value: value,
	}
	if space != nil {
		attr.Name.Space = *space
	}
	if o.Attributes == nil {
		o.Attributes = make([]*xml.Attr, 0)
	}
	o.Attributes = append(o.Attributes, &attr)
	return &attr
}

func (o *Element) ListElement(name string) []*Element {
	result := make([]*Element, 0)
	if o.Children == nil {
		return result
	}
	for _, e := range o.Children {
		if e.XMLName.Local == name {
			result = append(result, e)
		}
	}
	return result
}

func (o *Element) FindElement(path string) *Element {
	return FindElement(o, path)
}

func (o *Element) FindText(path string) *string {
	e := FindElement(o, path)
	if e == nil {
		return nil
	} else {
		return &e.Text
	}
}

func (o *Element) FindInt64(path string) *int64 {
	e := FindElement(o, path)
	if e == nil {
		return nil
	} else {
		return e.Int64()
	}
}

func (o *Element) FindFloat64(path string) *float64 {
	e := FindElement(o, path)
	if e == nil {
		return nil
	} else {
		return e.Float64()
	}
}

func (o *Element) FindTime(path string) *time.Time {
	e := FindElement(o, path)
	if e == nil {
		return nil
	} else {
		return e.Time()
	}
}

func (o *Element) Int64() *int64 {
	text := strings.TrimSpace(o.Text)
	if len(text) == 0 {
		return nil
	} else {
		v := util.ParseInt(text)
		return &v
	}
}

func (o *Element) Float64() *float64 {
	text := strings.TrimSpace(o.Text)
	if len(text) == 0 {
		return nil
	} else {
		v, e := strconv.ParseFloat(text, 64)
		util.CheckErr(e)
		return &v
	}
}

func (o *Element) Time() *time.Time {
	text := strings.TrimSpace(o.Text)
	if len(text) == 0 {
		return nil
	} else {
		v, e := time.Parse(time.RFC3339, text)
		util.CheckErr(e)
		return &v
	}
}

func MarshalPretty(o interface{}) []byte {
	buffer := &bytes.Buffer{}
	encoder := xml.NewEncoder(buffer)
	encoder.Indent("", "    ")
	util.CheckErr(encoder.Encode(o))
	return buffer.Bytes()
}

func Unmarshal(reader io.Reader, o interface{}) {
	decoder := xml.NewDecoder(reader)
	util.CheckErr(decoder.Decode(o))
}

func FindElement(root *Element, path string) *Element {
	steps := strings.Split(path, ".")
	current := root
	for _, key := range steps {
		if strings.HasPrefix(key, "#") {
			list := current.Children
			index := int(util.ParseInt(key[1:]))
			if index >= len(list) {
				return nil
			} else {
				current = list[index]
			}
		} else {
			object := current
			value := ChildElement(object, key)
			if value != nil {
				current = value
			} else {
				return nil
			}
		}
	}
	return current
}

func ChildElement(parent *Element, name string) *Element {
	if parent.Children == nil {
		return nil
	}
	for _, e := range parent.Children {
		if e.XMLName.Local == name {
			return e
		}
	}
	return nil
}
