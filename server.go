package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
	"github.com/lib/pq"
)

type Product struct {
	ItemID     int      `json:"Item_id"`
	ItemName   string   `json:"Item_name"`
	ItemDesc   string   `json:"Item_desc"`
	ItemPrice  float64  `json:"Item_price"`
	ItemSeller string   `json:"Item_seller"`
	ImageURL   []string `json:"Image_url"`
	Categories []string `json:"categories"`
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

	app.Put("/update", func(c *fiber.Ctx) error {
		return putHandler(c, db)
	})

	app.Delete("/delete", func(c *fiber.Ctx) error {
		return deleteHandler(c, db)
	})
	app.Post("/upload", func(c *fiber.Ctx) error {
		return postHandler(c, db)
	})

	app.Get("/product/:id", func(c *fiber.Ctx) error {
		return getProductHandler(c, db)
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
    // Execute SQL query to fetch all products
    rows, err := db.Query("SELECT Item_id, Item_name, Item_desc, Item_price, seller, COALESCE(image_url::text, '[]'), COALESCE(categories::text, '[]') FROM products")
    if err != nil {
        return c.Status(500).SendString("Failed to execute query: " + err.Error())
    }
    defer rows.Close()

    // Initialize an empty slice to store products
    var items []Product

    // Iterate over the rows returned by the query
    for rows.Next() {
        var item Product
        var imageUrlsJSON, categoriesJSON string

        // Scan values from the current row into variables
        if err := rows.Scan(&item.ItemID, &item.ItemName, &item.ItemDesc, &item.ItemPrice, &item.ItemSeller, &imageUrlsJSON, &categoriesJSON); err != nil {
            return c.Status(500).SendString("Failed to scan row: " + err.Error())
        }

        // Unmarshal JSON strings into slices
        if err := json.Unmarshal([]byte(imageUrlsJSON), &item.ImageURL); err != nil {
            return c.Status(500).SendString("Failed to unmarshal image URLs: " + err.Error())
        }

        // Parse PostgreSQL array format into []string
        item.Categories = parsePostgresArray(categoriesJSON)

        // Append the populated product struct to the items slice
        items = append(items, item)
    }

    // Return the items slice as JSON response
    return c.JSON(items)
}


func postHandler(c *fiber.Ctx, db *sql.DB) error {
	// Parse the file from the request
	form, err := c.MultipartForm()
	if err != nil {
		return c.Status(400).SendString("File upload error: " + err.Error())
	}

	files := form.File["files"]
	if len(files) == 0 {
		return c.Status(400).SendString("No files uploaded")
	}

	var imageUrls []string

	for _, file := range files {
		fileContent, err := file.Open()
		if err != nil {
			return c.Status(500).SendString("Failed to open file: " + err.Error())
		}
		defer fileContent.Close()

		s3URL, err := uploadToS3(file.Filename, fileContent)
		if err != nil {
			return c.Status(500).SendString("S3 upload error: " + err.Error())
		}
		imageUrls = append(imageUrls, s3URL)
	}

	// Marshal imageUrls into JSON
	imageUrlsJSON, err := json.Marshal(imageUrls)
	if err != nil {
		return c.Status(500).SendString("Failed to marshal image URLs: " + err.Error())
	}

	// Parse other form fields
	itemName := c.FormValue("item_name")
	itemDesc := c.FormValue("item_desc")
	itemSeller := c.FormValue("item_seller")
	itemPrice, err := strconv.ParseFloat(c.FormValue("item_price"), 64)
	if err != nil {
		return c.Status(400).SendString("Invalid item price")
	}

	categoriesJSON := c.FormValue("categories")
	var categories []string
	if err := json.Unmarshal([]byte(categoriesJSON), &categories); err != nil {
		return c.Status(400).SendString("Invalid categories format: " + err.Error())
	}

	// Insert record into the database
	sqlStatement := `
        INSERT INTO products (Item_name, Item_desc, Item_price, seller, image_url, categories)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING Item_id`
	var itemID int
	err = db.QueryRow(sqlStatement, itemName, itemDesc, itemPrice, itemSeller, string(imageUrlsJSON), pq.Array(categories)).Scan(&itemID)
	if err != nil {
		return c.Status(500).SendString("Database insert error: " + err.Error())
	}

	// Prepare response JSON
	response := map[string]interface{}{
		"Item_id":     itemID,
		"Item_name":   itemName,
		"Item_desc":   itemDesc,
		"Item_price":  itemPrice,
		"Item_seller": itemSeller,
		"categories":  categories, // Ensure categories is returned in the response
		"image_url":   imageUrls,
	}

	return c.JSON(response)
}

func putHandler(c *fiber.Ctx, db *sql.DB) error {
	type Item struct {
		ItemID     int      `json:"item_id"`
		ItemName   string   `json:"item_name"`
		ItemDesc   string   `json:"item_desc"`
		ItemPrice  float64  `json:"item_price"`
		ItemSeller string   `json:"item_seller"`
		Categories []string `json:"categories"`
	}

	item := new(Item)
	if err := c.BodyParser(item); err != nil {
		return c.Status(400).SendString(err.Error())
	}

	sqlStatement := `
		UPDATE products
		SET Item_name = $2, Item_desc = $3, Item_price = $4, Item_seller = $5, categories = $6
		WHERE Item_id = $1`
	res, err := db.Exec(sqlStatement, item.ItemID, item.ItemName, item.ItemDesc, item.ItemPrice, item.ItemSeller, pq.Array(item.Categories))
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
		"Categories":  item.Categories,
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
func getProductHandler(c *fiber.Ctx, db *sql.DB) error {
	productID := c.Params("id")
	if productID == "" {
		return c.Status(400).SendString("Product ID is required")
	}

	var product Product
	var imageUrlsJSON, categoriesJSON string
	sqlStatement := `
        SELECT Item_id, Item_name, Item_desc, Item_price, seller, COALESCE(image_url::text, '[]'), COALESCE(categories::text, '{}')
        FROM products
        WHERE Item_id = $1`
	err := db.QueryRow(sqlStatement, productID).Scan(&product.ItemID, &product.ItemName, &product.ItemDesc, &product.ItemPrice, &product.ItemSeller, &imageUrlsJSON, &categoriesJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return c.Status(404).SendString("Product not found")
		}
		return c.Status(500).SendString("Database query error: " + err.Error())
	}

	if err := json.Unmarshal([]byte(imageUrlsJSON), &product.ImageURL); err != nil {
		return c.Status(500).SendString("Failed to unmarshal image URLs: " + err.Error())
	}

	// Handle categories PostgreSQL array format
	categoriesArray := parsePostgresArray(categoriesJSON)

	product.Categories = categoriesArray

	return c.JSON(product)
}

// Helper function to parse PostgreSQL array format into []string
func parsePostgresArray(input string) []string {
	// Remove leading and trailing curly braces
	input = strings.Trim(input, "{}")
	// Split by commas
	categories := strings.Split(input, ",")
	// Trim whitespace from each element
	for i := range categories {
		categories[i] = strings.TrimSpace(categories[i])
	}
	return categories
}
