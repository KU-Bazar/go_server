package main

import (
    "database/sql"
    "fmt"
    "log"
    "os"
    "strconv"

    "github.com/gofiber/fiber/v2"
    "github.com/joho/godotenv"
    _ "github.com/lib/pq"
)

func main() {
    // Load environment variables from .env file
    err := godotenv.Load()
    if err != nil {
        log.Fatalf("Error loading .env file: %v", err)
    }

    // Get environment variables
    dbHost := os.Getenv("DB_HOST")
    dbPort := os.Getenv("DB_PORT")
    dbUser := os.Getenv("DB_USER")
    dbPassword := os.Getenv("DB_PASSWORD")
    dbName := os.Getenv("DB_NAME")

    // Create connection string
    connStr := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=require", dbUser, dbPassword, dbHost, dbPort, dbName)

    // Connect to the database
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        log.Fatal(err)
    }

    err = db.Ping()
    if err != nil {
        log.Fatal("Unable to connect to the database:", err)
    }

	
    app := fiber.New()

    // routes
    app.Get("/", func(c *fiber.Ctx) error {
        return indexHandler(c, db)
    })

    app.Post("/", func(c *fiber.Ctx) error {
        return postHandler(c, db)
    })

    app.Put("/update", func(c *fiber.Ctx) error {
        return putHandler(c, db)
    })

    app.Delete("/delete", func(c *fiber.Ctx) error {
        return deleteHandler(c, db)
    })

    // Port connection
    port := os.Getenv("PORT")
    if port == "" {
        port = "3000"
    }

    log.Fatalln(app.Listen(fmt.Sprintf(":%v", port)))
}

// Handler functions

func indexHandler(c *fiber.Ctx, db *sql.DB) error {
	rows, err := db.Query("SELECT Item_id, Item_name, Item_desc, Item_price FROM products")
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	defer rows.Close()

    var items []map[string]interface{}
	for rows.Next() {
		var itemID int
		var itemName, itemDesc string
		var itemPrice float64
		if err := rows.Scan(&itemID, &itemName, &itemDesc, &itemPrice); err != nil {
			return c.Status(500).SendString(err.Error())
		}
		item := map[string]interface{}{
			"Item_id":    itemID,
			"Item_name":  itemName,
			"Item_desc":  itemDesc,
			"Item_price": itemPrice,
		}
		items = append(items, item)
	}

	return c.JSON(items)
}


func postHandler(c *fiber.Ctx, db *sql.DB) error {
	type Item struct {
		ItemName  string  `json:"item_name"`
		ItemDesc  string  `json:"item_desc"`
		ItemPrice float64 `json:"item_price"`
	}

	item := new(Item)
	if err := c.BodyParser(item); err != nil {
		return c.Status(400).SendString(err.Error())
	}
// advantages && disadvantages for decoupled systems

	sqlStatement := `
		INSERT INTO products (Item_name, Item_desc, Item_price)
		VALUES ($1, $2, $3)
		RETURNING Item_id`
	var itemID int
	err := db.QueryRow(sqlStatement, item.ItemName, item.ItemDesc, item.ItemPrice).Scan(&itemID)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	return c.JSON(fiber.Map{
		"Item_id":    itemID,
		"Item_name":  item.ItemName,
		"Item_desc":  item.ItemDesc,
		"Item_price": item.ItemPrice,
	})
}

func putHandler(c *fiber.Ctx, db *sql.DB) error {
	type Item struct {
		ItemID    int     `json:"item_id"`
		ItemName  string  `json:"item_name"`
		ItemDesc  string  `json:"item_desc"`
		ItemPrice float64 `json:"item_price"`
	}

	item := new(Item)
	if err := c.BodyParser(item); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	sqlStatement := `
		UPDATE products
		SET Item_name = $2, Item_desc = $3, Item_price = $4
		WHERE Item_id = $1`
	res, err := db.Exec(sqlStatement, item.ItemID, item.ItemName, item.ItemDesc, item.ItemPrice)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	if rowsAffected == 0 {
		return c.Status(404).SendString("Item not found")
	}

	return c.JSON(fiber.Map{
		"Item_id":    item.ItemID,
		"Item_name":  item.ItemName,
		"Item_desc":  item.ItemDesc,
		"Item_price": item.ItemPrice,
	})
}

func deleteHandler(c *fiber.Ctx, db *sql.DB) error {
	itemID, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		return c.Status(400).SendString("Invalid item ID")
	}

	sqlStatement := `DELETE FROM products WHERE Item_id = $1`
	res, err := db.Exec(sqlStatement, itemID)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	if rowsAffected == 0 {
		return c.Status(404).SendString("Item not found")
	}

	return c.SendString("Item deleted")
}