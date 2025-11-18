package sql

import (
	"database/sql"

	"sparrowhawktech/toolkit/util"

	_ "github.com/lib/pq"
)

type Sequence struct {
	Name   string
	LastId int64
}

type SequenceProvider interface {
	next(string) int64
}

type PgSequenceProvider struct {
	SequenceProvider
	tx      *sql.Tx
	stmtMap map[string]*sql.Stmt
	nameMap map[string]string
}

func (o *PgSequenceProvider) next(name string) int64 {
	stmt := o.stmtMap[name]
	sequenceName, ok := o.nameMap[name]
	if !ok {
		sequenceName = name + "seq"
		o.nameMap[name] = sequenceName
	}
	if stmt == nil {

		var err error
		stmt, err = o.tx.Prepare("select * from nextval($1)")
		util.CheckErr(err)
		o.stmtMap[name] = stmt
	}
	r := QueryStmt(stmt, sequenceName)
	defer closeRows(r)
	var id int64
	r.Next()
	util.CheckErr(r.Scan(&id))
	return id
}

func NewPgSequenceProvider(tx *sql.Tx) *PgSequenceProvider {
	return &PgSequenceProvider{tx: tx, stmtMap: make(map[string]*sql.Stmt), nameMap: make(map[string]string)}
}

type Sequences struct {
	datasourceConfig DatasourceConfig
	provider         SequenceProvider
}

func (o *Sequences) Next(name string) int64 {
	return o.provider.next(name)
}

func NewSequences(config DatasourceConfig, tx *sql.Tx) *Sequences {
	manager := NewPgSequenceProvider(tx)
	return &Sequences{datasourceConfig: config, provider: manager}
}
