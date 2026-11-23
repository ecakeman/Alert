// Package postgres 实现 application/domain 定义的持久化端口（真实 PostgreSQL / pgx）。
// pool.go 是唯一的连接池装配入口；本包不得被 application / transport 反向引用。
package postgres