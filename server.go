package main

import (
    "database/sql"
    "fmt"
    "log"
    "os"
	"strconv"
    "github.com/gofiber/fiber/v2"
    _ "github.com/lib/pq"
)

func main() {
    // connection
    connStr := "postgresql://postgres:supauser@127.0.0.1/kubazar?sslmode=disable"

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
	rows, err := db.Query("SELECT Item_id, Item_name, Item_desc, Item_price FROM kubazar")
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

	sqlStatement := `
		INSERT INTO kubazar (Item_name, Item_desc, Item_price)
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
		UPDATE kubazar
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

	sqlStatement := `DELETE FROM kubazar WHERE Item_id = $1`
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