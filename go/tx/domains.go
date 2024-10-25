package tx

import (
	"sparrowhawktech/toolkit/sql"
)

type Domain struct {
	transaction *Transaction
	schema      string
	name        string
	idMap       map[string]int64
	codeMap     map[int64]string
}

func (o *Domain) load() {
	o.codeMap = make(map[int64]string)
	o.idMap = make(map[string]int64)
	r := o.transaction.Query("select id, " + o.name + "code from " + o.schema + "." + o.name)
	defer r.Close()
	for r.Next() {
		var id int64
		var code string
		sql.Scan(r, &id, &code)
		o.codeMap[id] = code
		o.idMap[code] = id
	}
}

func (o *Domain) FindId(code string) *int64 {
	if o.idMap == nil {
		o.load()
	}
	v, ok := o.idMap[code]
	if ok {
		return &v
	} else {
		return nil
	}
}

func (o *Domain) FindCode(id int64) *string {
	if o.codeMap == nil {
		o.load()
	}
	v, ok := o.codeMap[id]
	if ok {
		return &v
	} else {
		return nil
	}
}

func NewDomain(tx *Transaction, schema string, name string) *Domain {
	return &Domain{schema: schema, name: name, transaction: tx}
}
