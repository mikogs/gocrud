# Demo

This demo walks you through setting up a complete, runnable example. We will define a model, initialize the controller, create the database table, and perform Create, Read, Update, List, and Delete operations.

## 1. Define Your Model and Entry Point

Start by defining your `User` struct and wrapping the logic in a standard Go main function. This ensures the code is executable and demonstrates how the model integrates with the application lifecycle.

```go
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Import your database driver here (e.g., github.com/lib/pq or modernc.org/sqlite)
	// "database/sql"
	// _ "your-db-driver"
	
	"github.com/mikogs/gocrud"
	// Import the filters package for advanced querying
	sqlfilters "github.com/mikogs/gocrud/pkg/filters"
)

// User represents the data model to be stored in the database.
type User struct {
	ID         uint64 `json:"id"`
	Username   string `json:"username" validate:"required,min=3,max=30" crud_regexp:"^[a-z0-9_-]+$"`
	Email      string `json:"email" validate:"required,email"`
	FirstName  string `json:"first_name" validate:"required,min=2,max=50"`
	LastName   string `json:"last_name" validate:"required,min=2,max=50"`
	Phone      string `json:"phone" validate:"omitempty,min=7,max=20"`
	Department string `json:"department" validate:"max=100"`
	Role       string `json:"role" validate:"required,min=3,max=30"`
	IsActive   bool   `json:"is_active"`
	LastLogin  int64  `json:"last_login" validate:"min=0"`
	CreatedAt  int64  `json:"created_at"`
	CreatedBy  uint64 `json:"created_by"`
	ModifiedAt int64  `json:"modified_at"`
	ModifiedBy uint64 `json:"modified_by"`
}

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	ctx := context.Background()

	// NOTE: Replace this with your actual database connection logic.
	// dbConn, err := sql.Open("postgres", "user=... dbname=...")
	// if err != nil {
	//     slog.Error("failed to connect to DB", slog.Any("error", err))
	//     os.Exit(1)
	// }
	var dbConn interface{} // Replace with *sql.DB in real usage

	// 2. Initialize the CRUD Controller
	crudInstance := gocrud.New(dbConn, gocrud.Options{
		Dialect: gocrud.DialectPostgres,
	})

	// 3. Create the Database Table
	err := crudInstance.CreateTable(ctx, &User{})
	if err != nil {
		slog.Error("error creating table", slog.Any("error", err))
		os.Exit(1)
	}

	// 4. Perform CRUD Operations
	runOperations(ctx, crudInstance)
}
```

A few important conventions:

* Every struct must include an `ID` field of type `uint64`. This maps to a BIGINT primary key in the database.
* Fields tracking creation and modification (`CreatedAt`, `CreatedBy`, `ModifiedAt`, `ModifiedBy`) are recognized automatically. When present, gocrud populates their values on insert and update. 
* Validation rules are expressed via the `validate` tag (and `crud_regexp` for regex patterns). `gocrud` leverages the [`go-playground/validator` library](https://github.com/go-playground/validator) under the hood—refer to its documentation for the full list of supported validators. 
* At present, `gocrud` is optimized for rapid prototyping and supports only `int`, `uint`, `float`, `string`, and `bool` field types.


## 2. Initialize the CRUD Controller

Import the gocrud package and create a controller instance tied to your database connection. As shown in the main function above:

```go
crudInstance := gocrud.New(dbConn, gocrud.Options{
    Dialect: gocrud.DialectPostgres,
})
```

Replace `dbConn` with your active database connection (for example, a `*sql.DB `or compatible interface expected by your setup).

## 3. Create the Database Table

With the controller ready, instruct gocrud to create the corresponding table if it doesn't already exist:

```go
err := crudInstance.CreateTable(ctx, &User{})
if err != nil {
    slog.Error("error creating table", slog.Any("error", err))
    os.Exit(1)
}
```

`CreateTable` inspects the `User` struct via reflection, interprets the field tags, and generates the necessary `CREATE TABLE IF NOT EXISTS SQL` statement. This ensures your schema aligns with the struct definition without manual SQL.

## 4. Perform CRUD Operations

Now let's put the controller to work. We'll create several users, read one back, update it, list users with advanced filtering, and finally delete one.
Each operation is demonstrated separately below. All helper functions are called from runOperations() which is invoked from main().

```go
func runOperations(ctx context.Context, c *gocrud.Controller) {
	createUsers(ctx, c)
	loadUser(ctx, c)
	updateUser(ctx, c)
	listUsers(ctx, c)
	deleteUser(ctx, c)
}
```

### Create — Insert New Records

Use the `Save` method to insert new records. The `ID` is auto-generated and populated on the struct after saving.

```go
func createUsers(ctx context.Context, c *gocrud.Controller) {
	now := time.Now().Unix()
	userID := uint64(1337)

	// Iterate 5 times to create and save users
	for i := 0; i < 5; i++ {
		userInstance := &User{
			Username:   fmt.Sprintf("user_%d", i),
			Email:      fmt.Sprintf("user%d@example.com", i),
			FirstName:  "Demo",
			LastName:   "User",
			Role:       "admin",
			IsActive:   true,
			Department: "Engineering",
			Phone:      "1234567890",
		}

		err := c.crud.Save(ctx, userInstance, gocrud.SaveOptions{
			ModifiedAt: now,
			ModifiedBy: userID,
		})
		if err != nil {
			slog.Error("failed to save user", slog.Int("index", i), slog.Any("error", err))
			os.Exit(1)
		}

		slog.Info("saved user", slog.String("username", userInstance.Username), slog.Uint64("id", userInstance.ID))
	}
}
```

### Read — Load a Single Record by ID

Use the `Load` method to fetch a single record by its primary key into an empty struct.

```go
func loadUser(ctx context.Context, c *gocrud.Controller) {
	userFromDB := &User{}
	output := c.crud.Load(ctx, userFromDB, 2, gocrud.LoadOptions{})
	if output.Error != nil {
		slog.Error("failed to load user", slog.Any("error", output.Error))
		os.Exit(1)
	}
	slog.Info("loaded user", slog.String("username", userFromDB.Username), slog.Uint64("id", userFromDB.ID))
}
```

### Update — Modify Existing Records

Call `Save` on an existing (loaded) struct with modified fields. This updates the record in place.

```go
func updateUser(ctx context.Context, c *gocrud.Controller) {
	now := time.Now().Unix()
	userID := uint64(1337)

	// First load the user
	userFromDB := &User{}
	output := c.crud.Load(ctx, userFromDB, 2, gocrud.LoadOptions{})
	if output.Error != nil {
		slog.Error("failed to load user", slog.Any("error", output.Error))
		os.Exit(1)
	}

	// Modify a field
	userFromDB.LastName = "Updated"

	// Save the changes
	err := c.crud.Save(ctx, userFromDB, gocrud.SaveOptions{
		ModifiedAt: now,
		ModifiedBy: userID,
	})
	if err != nil {
		slog.Error("failed to update user", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("updated user", slog.String("lastName", userFromDB.LastName))
}
```

### List — Fetch Multiple Records with Filters

Use the `Get` method to retrieve multiple records with pagination, sorting, and advanced filtering.

**Understanding GetOptions**:

|Field	|Description|
|-------|-----------|
|Order	|Slice of strings specifying sort columns (e.g., `[]string{"Username", "-CreatedAt"}`)|
|Limit	|Maximum number of records to return|
|Offset	|Number of records to skip (pagination)|
|Filters	|Complex filtering logic (equality, ranges, raw SQL)|
|RowObjTransformFunc	|Transform each row into any type (e.g., string, HTML, CSV)|
|ConvertFiltersFromString	|Auto-convert string filter values to target types|

**Note**: To use filters, import: `sqlfilters "github.com/mikogs/gocrud/pkg/filters"`

```go
func listUsers(ctx context.Context, c *gocrud.Controller) {
	fetchedUsers, err := c.crud.Get(ctx, func() { &User{} }, gocrud.GetOptions{
		Limit:  10,
		Offset: 0,
		Order:  []string{"Username"},
		Filters: &sqlfilters.Filters{
			// Filter by Department (String)
			"Department": {
				Op:  sqlfilters.OpEqual,
				Val: "Engineering",
			},
			// Filter by IsActive (Bool)
			"IsActive": {
				Op:  sqlfilters.OpEqual,
				Val: true,
			},
			// Raw SQL filter: ID > 0 AND ID NOT IN (9999, 9998, 9997)
			// Note: Use ".FieldName" to reference columns safely
			sqlfilters.Raw: {
				Op: sqlfilters.OpAND,
				Val: []interface{}{
					".ID > ? AND .ID NOT IN (?)",
					0,
					[]int{9999, 9998, 9997},
				},
			},
		},
	})
	if err != nil {
		slog.Error("failed to list users", slog.Any("error", err))
		os.Exit(1)
	}
	fmt.Printf("fetched %d filtered users\n", len(fetchedUsers))
}
```

### Delete — Remove Records

Load a user and then call `Delete` to remove it from the database.

```go
func deleteUser(ctx context.Context, c *gocrud.Controller) {
	userToDelete := &User{}
	output := c.crud.Load(ctx, userToDelete, 2, gocrud.LoadOptions{})
	if output.Error != nil {
		slog.Error("failed to load user for deletion", slog.Any("error", output.Error))
		os.Exit(1)
	}

	err := c.crud.Delete(ctx, userToDelete, gocrud.DeleteOptions{})
	if err != nil {
		slog.Error("failed to delete user", slog.Any("error", err))
		os.Exit(1)
	}
	slog.Info("deleted user", slog.Uint64("id", userToDelete.ID))
}
```

### Operation Summary

|Operation|Method|Purpose|
|---------|------|-------|
|Create	  |Save	 |Inserts a new record. The ID is auto-generated and populated on the struct after saving.|
|Read	  |Load	 |Fetches a single record by its primary key into an empty struct.|
|Update	  |Save	 |Called on an existing (loaded) struct with modified fields. Updates the record in place.|
|List	  |Get	 |Retrieves multiple records with pagination (Limit, Offset) and ordering.|
|Delete	  |Delete|Removes the record corresponding to the loaded struct from the database.|

