package sql

import (
	"database/sql"

	_ "github.com/lib/pq"
	"sparrowhawktech/toolkit/util"
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
}

func (o *PgSequenceProvider) next(name string) int64 {
	stmt := o.stmtMap[name]
	if stmt == nil {
		var err error
		stmt, err = o.tx.Prepare("select * from nextval('" + name + "seq')")
		util.CheckErr(err)
		o.stmtMap[name] = stmt
	}
	r := QueryStmt(stmt)
	defer closeRows(r)
	var id int64
	r.Next()
	util.CheckErr(r.Scan(&id))
	return id
}

func NewPgSequenceProvider(tx *sql.Tx) *PgSequenceProvider {
	return &PgSequenceProvider{tx: tx, stmtMap: make(map[string]*sql.Stmt)}
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
