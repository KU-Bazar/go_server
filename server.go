package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type Product struct {
	ItemID     int     `json:"Item_id"`
	ItemName   string  `json:"Item_name"`
	ItemDesc   string  `json:"Item_desc"`
	ItemPrice  float64 `json:"Item_price"`
	ItemSeller string  `json:"Item_seller"`
	ImageURL   string  `json:"image_url"`
}

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
	app.Post("/upload", func(c *fiber.Ctx) error {
		return uploadHandler(c, db)
	})

	// Port connection
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}

	log.Fatalln(app.Listen(fmt.Sprintf(":%v", port)))
}

// Handler functions
func uploadToS3(filename string, fileContent io.Reader) (string, error) {
	bucket := os.Getenv("S3_BUCKET_NAME")
	region := os.Getenv("AWS_REGION")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	// Initialize a session using Amazon S3
	sess, err := session.NewSession(&aws.Config{
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
	})
	if err != nil {
		return "", err
	}

	// Upload the file to S3
	svc := s3.New(sess)
	key := fmt.Sprintf("uploads/%d-%s", time.Now().Unix(), filepath.Base(filename))

	buffer := new(bytes.Buffer)
	_, err = io.Copy(buffer, fileContent)
	if err != nil {
		return "", err
	}

	_, err = svc.PutObject(&s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(buffer.Bytes()),
	})
	if err != nil {
		return "", fmt.Errorf("failed to put object in S3: %v", err)
	}

	// Return the URL of the uploaded file
	url := fmt.Sprintf("https://%s.s3-%s.amazonaws.com/%s", bucket, region, key)
	return url, nil
}

func indexHandler(c *fiber.Ctx, db *sql.DB) error {
	rows, err := db.Query("SELECT Item_id, Item_name, Item_desc, Item_price, seller, COALESCE(Image_url, '') as Image_url FROM products")
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}
	defer rows.Close()

	var items []Product
	for rows.Next() {
		var item Product
		if err := rows.Scan(&item.ItemID, &item.ItemName, &item.ItemDesc, &item.ItemPrice, &item.ItemSeller, &item.ImageURL); err != nil {
			return c.Status(500).SendString(err.Error())
		}
		items = append(items, item)
	}
	return c.JSON(items)
}


func postHandler(c *fiber.Ctx, db *sql.DB) error {
	// Parse the file from the request
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).SendString("File upload error: " + err.Error())
	}

	fileContent, err := file.Open()
	if err != nil {
		return c.Status(500).SendString("File open error: " + err.Error())
	}
	defer fileContent.Close()

	// Upload file to S3
	s3URL, err := uploadToS3(file.Filename, fileContent)
	if err != nil {
		return c.Status(500).SendString("S3 upload error: " + err.Error())
	}

	// Parse other form fields
	itemName := c.FormValue("item_name")
	itemDesc := c.FormValue("item_desc")
	itemSeller := c.FormValue(("item_seller"))
	itemPrice, err := strconv.ParseFloat(c.FormValue("item_price"), 64)
	if err != nil {
		return c.Status(400).SendString("Invalid item price")
	}

	// Insert record into the database
	sqlStatement := `
        INSERT INTO products (Item_name, Item_desc, Item_price, seller ,image_url)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING Item_id`
	var itemID int
	err = db.QueryRow(sqlStatement, itemName, itemDesc, itemPrice, itemSeller, s3URL).Scan(&itemID)
	if err != nil {
		return c.Status(500).SendString("Database insert error: " + err.Error())
	}

	return c.JSON(fiber.Map{
		"Item_id":     itemID,
		"Item_name":   itemName,
		"Item_desc":   itemDesc,
		"Item_price":  itemPrice,
		"Item_seller": itemSeller,
		"image_url":   s3URL,
	})
}
func uploadHandler(c *fiber.Ctx, db *sql.DB) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(400).SendString("Failed to get the file")
	}

	// Open the file
	fileContent, err := file.Open()
	if err != nil {
		return c.Status(500).SendString("Failed to open the file")
	}
	defer fileContent.Close()

	// Upload to S3
	imageUrl, err := uploadToS3(file.Filename, fileContent)
	if err != nil {
		return c.Status(500).SendString("Failed to upload the file to S3")
	}

	itemName := c.FormValue("item_name")
	itemDesc := c.FormValue("item_desc")
	itemPriceStr := c.FormValue("item_price")
	itemSeller := c.FormValue("item_seller")
	itemPrice, err := strconv.ParseFloat(itemPriceStr, 64)
	if err != nil {
		return c.Status(400).SendString("Invalid item price")
	}

	sqlStatement := `
        INSERT INTO products (Item_name, Item_desc, Item_price, seller ,image_url)
        VALUES ($1, $2, $3, $4, $5)
        RETURNING Item_id`
	var itemID int
	err = db.QueryRow(sqlStatement, itemName, itemDesc, itemPrice, itemSeller, imageUrl).Scan(&itemID)
	if err != nil {
		return c.Status(500).SendString(err.Error())
	}

	return c.JSON(fiber.Map{
		"Item_id":     itemID,
		"Item_name":   itemName,
		"Item_desc":   itemDesc,
		"Item_price":  itemPrice,
		"Item_seller": itemSeller,
		"Image_url":   imageUrl,
	})
}

func putHandler(c *fiber.Ctx, db *sql.DB) error {
	type Item struct {
		ItemID     int     `json:"item_id"`
		ItemName   string  `json:"item_name"`
		ItemDesc   string  `json:"item_desc"`
		ItemPrice  float64 `json:"item_price"`
		ItemSeller string  `json:"item_seller"`
	}

	item := new(Item)
	if err := c.BodyParser(item); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	sqlStatement := `
		UPDATE products
		SET Item_name = $2, Item_desc = $3, Item_price = $4, Item_seller = $4
		WHERE Item_id = $1`
	res, err := db.Exec(sqlStatement, item.ItemID, item.ItemName, item.ItemDesc, item.ItemPrice, item.ItemSeller)
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
		"Item_id":     item.ItemID,
		"Item_name":   item.ItemName,
		"Item_desc":   item.ItemDesc,
		"Item_price":  item.ItemPrice,
		"Item_seller": item.ItemSeller,
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
