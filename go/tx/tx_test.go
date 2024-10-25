package tx_test

import (
	sql2 "database/sql"
	"sparrowhawktech/toolkit/coverage"
	"sparrowhawktech/toolkit/sql"
	"sparrowhawktech/toolkit/tx"
	"sparrowhawktech/toolkit/util"
	"testing"
)

func TestSql(t *testing.T) {

	dataSourceConfig := sql.DatasourceConfig{
		DriverName:  util.PStr("postgres"),
		Name:        util.PStr("postgres://postgres:postgres@localhost?sslmode=disable"),
		MaxIdle:     nil,
		MaxOpen:     nil,
		MaxLifetime: nil,
	}

	coverage.ExecuteDB(dataSourceConfig, func(db *sql2.DB) {
		_, err := db.Exec(`drop database if exists "coverage-tx"`)
		util.CheckErr(err)
	})

	coverage.ExecuteDB(dataSourceConfig, func(db *sql2.DB) {
		_, err := db.Exec(`create database "coverage-tx"`)
		util.CheckErr(err)
	})

	dataSourceConfig.Name = util.PStr("postgres://postgres:postgres@localhost/coverage-tx?sslmode=disable")

	tx.Execute(dataSourceConfig, func(trx *tx.Transaction, args ...interface{}) interface{} {
		trx.Exec("create table test(id bigint)")
		trx.Exec("insert into test values(1)")
		return nil
	})

	tx.ExecuteRO(dataSourceConfig, func(trx *tx.Transaction, args ...interface{}) interface{} {
		rows := trx.Query("select * from test")
		sql.ScanAll(rows)
		return nil
	})

}
