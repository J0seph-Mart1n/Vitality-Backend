package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	vision "cloud.google.com/go/vision/apiv1"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	// "github.com/tmc/langchaingo/llms"
	// "github.com/tmc/langchaingo/llms/openai"
	// "github.com/tmc/langchaingo/prompts"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		// It's good practice to just log a warning rather than crash,
		// because in production (like Docker/Heroku/AWS), you might pass
		// actual environment variables instead of using a .env file.
		log.Println("Warning: Error loading .env file or .env file not found")
	} else {
		fmt.Println(".env file loaded successfully!")
	}

	// Create a new HTTP multiplexer (router)
	// Initialize Gin router
	r := gin.Default()

	// Define the API route
	r.POST("/analyze-label", handleImageUpload)

	// Start the server on port 8080
	fmt.Println("Server running on http://localhost:8080")
	r.Run(":8080")
}

func handleImageUpload(c *gin.Context) {
	// 1. Receive the image from the React Native app
	file, _, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get image from request"})
		return
	}
	defer file.Close()

	// 2. Extract Text using Google Cloud Vision
	extractedText, err := extractTextFromImage(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to extract text: " + err.Error()})
		return
	}

	// Check if any text was found
	if strings.TrimSpace(extractedText) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No text could be found in the image"})
		return
	}

	// 3. Analyze the extracted text using LangChain and LLM
	// analysisJSON, err := analyzeNutritionData(extractedText)
	// if err != nil {
	// 	c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to analyze text: " + err.Error()})
	// 	return
	// }

	// 4. Send the JSON response back to the app
	// Temporarily returning the raw text wrapped in a valid JSON object
	c.JSON(http.StatusOK, gin.H{
		"text": extractedText,
		"benefits": []string{"(LLM Analysis not active yet)"},
		"harmful_effects": []string{"(LLM Analysis not active yet)"},
	})
}

// extractTextFromImage sends the image stream to Google Cloud Vision API
func extractTextFromImage(file io.Reader) (string, error) {
	ctx := context.Background()

	// Creates a Google Vision client.
	// (Ensure GOOGLE_APPLICATION_CREDENTIALS env var is set)
	client, err := vision.NewImageAnnotatorClient(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to create vision client: %v", err)
	}
	defer client.Close()

	// Create vision Image from the file reader
	image, err := vision.NewImageFromReader(file)
	if err != nil {
		return "", fmt.Errorf("failed to read image: %v", err)
	}

	// Call Vision API to detect document text (optimized for dense text like labels)
	annotation, err := client.DetectDocumentText(ctx, image, nil)
	if err != nil {
		return "", fmt.Errorf("vision API error: %v", err)
	}

	if annotation == nil {
		return "", nil
	}

	return annotation.Text, nil
}
