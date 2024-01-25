package fluentmodel

import (
	"crypto/rand"
	"errors"
	"github.com/jiveio/fluentsql"
	"log"
	"math/big"
	"reflect"
)

// ===========================================================================================================
//										Query ONE row
// ===========================================================================================================

type GetOne int

const (
	GetFirst GetOne = iota
	GetLast
	TakeOne
)

// First get the first record ordered by primary key
//
//	Example
//
// -------- Query a First  --------
//
// var user User
// err = db.First(&user)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user)
//
// -------- Query a First by ID  --------
//
// var user3 User
// err = db.First(&user3, 103)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user3)
// var user4 User
// user4 = User{Id: 103}
// err = db.First(&user4)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user4)
//
// -------- Query a First by Model  --------
//
// var user5 User
// err = db.Model(User{Id: 102}).First(&user5)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user5)
//
// -------- Query a First by Where  --------
//
// var user6 User
// err = db.Where("Id", fluentsql.Eq, 100).First(&user6)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user6)
//
// -------- Query a First by WhereGroup  --------
//
// var user7 User
// err = db.Where("Id", fluentsql.Eq, 100).
//
//	WhereGroup(func(query fluentsql.WhereBuilder) *fluentsql.WhereBuilder {
//		query.Where("age", fluentsql.Eq, 42).
//			WhereOr("age", fluentsql.Eq, 39)
//
//		return &query
//	}).First(&user7)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user7)
func (db *DBModel) First(model any, args ...any) (err error) {
	if len(args) > 0 {
		db.wherePrimaryCondition = fluentsql.Condition{
			Field: nil,
			Opt:   fluentsql.Eq,
			Value: args[0],
			AndOr: fluentsql.And,
		}
	}

	err = db.GetOne(model, GetFirst)

	return
}

// Take get one record, no specified order
//
//	Example
//
// -------- Query a Take  --------
//
// var user2 User
// err = db.Take(&user2)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user2)
func (db *DBModel) Take(model any) (err error) {
	err = db.GetOne(model, TakeOne)

	return
}

// Last last record, ordered by primary key desc
//
//	Example
//
// -------- Query a Last  --------
//
// var user1 User
// err = db.Select("name").Last(&user1)
//
//	if err != nil {
//		log.Fatal(err)
//	}
//
// log.Printf("User %v\n", user1)
func (db *DBModel) Last(model any) (err error) {
	err = db.GetOne(model, GetLast)

	return
}

// GetOne with specific strategy GetLast | GetFirst | TakeOne
func (db *DBModel) GetOne(model any, getType GetOne) (err error) {
	// Query raw SQL
	if db.raw.sqlStr != "" {
		// Data persistence
		if db.tx != nil {
			err = db.tx.Get(model, db.raw.sqlStr, db.raw.args...)
		} else {
			err = dbInstance.Get(model, db.raw.sqlStr, db.raw.args...)
		}

		// Reset fluent model builder
		db.reset()

		return
	}

	typ := reflect.TypeOf(model)

	if !(typ.Kind() == reflect.Struct ||
		(typ.Kind() == reflect.Ptr && typ.Elem().Kind() == reflect.Struct)) {
		err = errors.New("invalid data :: model not Struct type")

		return
	}

	var table *Table
	var primaryKey any

	// Create a table object from a model
	table, err = ModelData(model)

	// Get a primary key
	if len(table.Primaries) > 0 {
		primaryKey = table.Primaries[0].Name
	}

	// Columns which will be queried
	var selectColumns []any

	// Only some selected columns or all
	if len(db.selectStatement.Columns) > 0 {
		selectColumns = db.selectStatement.Columns
	} else {
		selectColumns = []any{"*"}
	}

	// Create query builder
	queryBuilder := fluentsql.QueryInstance().
		Select(selectColumns...).
		From(table.Name).
		Limit(1, 0)

	// Build WHERE condition with specific primary value
	if db.wherePrimaryCondition.Value != nil && primaryKey != nil {
		queryBuilder.Where(primaryKey, db.wherePrimaryCondition.Opt, db.wherePrimaryCondition.Value)
	}

	// Build WHERE condition from a condition list
	for _, condition := range db.whereStatement.Conditions {
		// Sub-conditions
		if len(condition.Group) > 0 {
			// Append conditions from a group to query builder
			queryBuilder.WhereGroup(func(whereBuilder fluentsql.WhereBuilder) *fluentsql.WhereBuilder {
				whereBuilder.WhereCondition(condition.Group...)

				return &whereBuilder
			})
		} else if condition.AndOr == fluentsql.And {
			// Add Where AND condition
			queryBuilder.Where(condition.Field, condition.Opt, condition.Value)
		} else if condition.AndOr == fluentsql.Or {
			// Add Where OR condition
			queryBuilder.WhereOr(condition.Field, condition.Opt, condition.Value)
		}
	}

	// Build WHERE condition from model's data
	if table.HasData {
		for _, column := range table.Columns {
			if !column.HasValue {
				continue
			}

			// Append query conditions
			queryBuilder.Where(column.Name, fluentsql.Eq, table.Values[column.Name])
		}
	}

	// Build WHERE condition from a specific model
	db.whereFromModel(queryBuilder)

	orderByField := ""
	if primaryKey != nil {
		orderByField = primaryKey.(string)
	} else {
		orderByField = table.Columns[0].Name
	}

	var orderByDir fluentsql.OrderByDir

	// Get order
	if getType == GetLast && orderByField != "" {
		orderByDir = fluentsql.Desc
	} else if getType == GetFirst && orderByField != "" {
		orderByDir = fluentsql.Asc
	} else if getType == TakeOne { // Random order field and order dir
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(table.Columns)-1)))
		orderByField = table.Columns[n.Int64()].Name

		n, _ = rand.Int(rand.Reader, big.NewInt(10))
		if n.Int64()%2 == 1 {
			orderByDir = fluentsql.Asc
		} else {
			orderByDir = fluentsql.Desc
		}
	}

	// Build Order By clause
	queryBuilder.OrderBy(orderByField, orderByDir)

	// Data persistence
	err = db.get(queryBuilder, model)

	// Reset fluent model builder
	db.reset()

	return
}

// ===========================================================================================================
//										Query MULTI rows
// ===========================================================================================================

// Find search rows
func (db *DBModel) Find(model any, params ...any) (total int, err error) {
	// Query raw SQL
	if db.raw.sqlStr != "" {
		// Data persistence
		if db.tx != nil {
			err = db.tx.Select(model, db.raw.sqlStr, db.raw.args...)
		} else {
			err = dbInstance.Select(model, db.raw.sqlStr, db.raw.args...)
		}

		// Reset fluent model builder
		db.reset()

		return
	}

	typ := reflect.TypeOf(model)

	if !(typ.Kind() == reflect.Ptr && typ.Elem().Kind() == reflect.Slice) {
		err = errors.New("invalid data :: model not *Slice type")
		return
	}

	var table *Table
	var primaryKey any

	// Get a type of model and get an element
	typeElement := reflect.TypeOf(model).Elem().Elem()  // First Elem() for pointer. Second Elem() for item
	valueElement := reflect.ValueOf(typeElement).Elem() // Create empty value

	table = NewTable()
	table = processModel(typeElement, valueElement, table)

	// Get a primary key
	if len(table.Primaries) > 0 {
		primaryKey = table.Primaries[0].Name
	}

	if len(params) > 0 && primaryKey != nil {
		sliceIds := params[0]

		typ := reflect.TypeOf(sliceIds)
		if !(typ.Kind() == reflect.Slice) {
			err = errors.New("invalid data :: params not Slice type")
			return
		}

		db.wherePrimaryCondition = fluentsql.Condition{
			Field: primaryKey,
			Opt:   fluentsql.In,
			Value: sliceIds,
			AndOr: fluentsql.And,
		}
	}

	// Columns which will be queried
	var selectColumns []any

	// Only some selected columns or all
	if len(db.selectStatement.Columns) > 0 {
		selectColumns = db.selectStatement.Columns
	} else {
		selectColumns = []any{"*"}
	}

	// Create query builder
	queryBuilder := fluentsql.QueryInstance().
		Select(selectColumns...).
		From(table.Name)

	// Build JOIN clause
	for _, joinItem := range db.joinStatement.Items {
		queryBuilder.Join(joinItem.Join, joinItem.Table, joinItem.Condition)
	}

	// Build WHERE condition with specific primary value
	if db.wherePrimaryCondition.Value != nil && primaryKey != nil {
		log.Printf("%v", db.wherePrimaryCondition.Value)
		queryBuilder.WhereCondition(db.wherePrimaryCondition)
	}

	// Build WHERE condition from a condition list
	for _, condition := range db.whereStatement.Conditions {
		// Sub-conditions
		if len(condition.Group) > 0 {
			// Append conditions from a group to query builder
			queryBuilder.WhereGroup(func(whereBuilder fluentsql.WhereBuilder) *fluentsql.WhereBuilder {
				whereBuilder.WhereCondition(condition.Group...)

				return &whereBuilder
			})
		} else if condition.AndOr == fluentsql.And {
			// Add Where AND condition
			queryBuilder.Where(condition.Field, condition.Opt, condition.Value)
		} else if condition.AndOr == fluentsql.Or {
			// Add Where OR condition
			queryBuilder.WhereOr(condition.Field, condition.Opt, condition.Value)
		}
	}

	// Build WHERE condition from a specific model
	db.whereFromModel(queryBuilder)

	// Build GROUP BY clause
	if len(db.groupByStatement.Items) > 0 {
		queryBuilder.GroupBy(db.groupByStatement.Items...)
	}

	// Build HAVING clause
	for _, condition := range db.havingStatement.Conditions {
		queryBuilder.Having(condition.Field, condition.Opt, condition.Value)
	}

	// Build LIMIT clause
	if db.limitStatement.Limit > 0 {
		queryBuilder.Limit(db.limitStatement.Limit, db.limitStatement.Offset)
	}

	// Build FETCH clause
	if db.fetchStatement.Fetch > 0 {
		queryBuilder.Fetch(db.fetchStatement.Offset, db.fetchStatement.Fetch)
	}

	// Build ORDER BY clause
	for _, orderItem := range db.orderByStatement.Items {
		queryBuilder.OrderBy(orderItem.Field, orderItem.Direction)
	}

	// Data persistence
	if err = db.query(queryBuilder, model); err != nil {
		return
	}

	if err = db.count(queryBuilder, &total); err != nil {
		return
	}

	// Reset fluent model builder
	db.reset()

	return
}
