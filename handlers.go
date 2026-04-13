package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func handleImageUpload(c *gin.Context) {
	// 1. Receive the image from the React Native app
	file, _, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to get image from request"})
		return
	}
	defer file.Close()

	// 2. Convert the image directly to a Base64 string
	base64Image, err := encodeImageToBase64(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode image: " + err.Error()})
		return
	}

	// 3. Send the Base64 image + Prompt directly to Groq (Llama 4 Scout)
	analysisJSON, err := analyzeImageWithGroq(base64Image)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to analyze image: " + err.Error()})
		return
	}

	// 4. Safely parse the LLM's JSON to ensure it is 100% valid before sending it to the app
	var parsedData map[string]interface{}
	if err := json.Unmarshal([]byte(analysisJSON), &parsedData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LLM returned invalid JSON format", "raw_output": analysisJSON})
		return
	}

	// 5. Send the perfectly validated JSON back to the app!
	c.JSON(http.StatusOK, parsedData)
}

func handleEstimateNutrition(c *gin.Context) {
	var req EstimateNutritionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload format"})
		return
	}

	analysisJSON, err := estimateNutritionWithGroq(req.FoodName, req.NutritionalFacts, req.ConsumedAmount, req.Calories, req.Protein, req.Carbs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to estimate nutrition: " + err.Error()})
		return
	}

	var parsedData map[string]interface{}
	if err := json.Unmarshal([]byte(analysisJSON), &parsedData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "LLM returned invalid JSON format", "raw_output": analysisJSON})
		return
	}

	c.JSON(http.StatusOK, parsedData)
}

func handleSaveScan(c *gin.Context) {
	// 1. Verify User
	uid, exists := c.Get("UID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// 2. Parse the incoming JSON data from the React Native app
	var req SaveScanRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload format"})
		return
	}

	// 3. Create the Database Document
	newScan := ScannedLabel{
		UserID:    uid.(string), // From Firebase Token
		Data:      req.Data,     // The JSON data sent from the frontend
		CreatedAt: time.Now(),
	}

	// 4. Save to MongoDB
	_, err := labelCollection.InsertOne(context.Background(), newScan)
	if err != nil {
		log.Printf("Failed to save to MongoDB: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save scan to database"})
		return
	}

	// 5. Success!
	c.JSON(http.StatusOK, gin.H{"message": "Scan saved successfully!"})
}

func handleGetScanHistory(c *gin.Context) {
	uid, exists := c.Get("UID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// Filter by current user
	filter := bson.M{"user_id": uid.(string)}
	// Sort by created_at descending (newest first)
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := labelCollection.Find(context.Background(), filter, opts)
	if err != nil {
		log.Printf("Error fetching scan history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch scan history"})
		return
	}
	defer cursor.Close(context.Background())

	var scans []ScannedLabel = make([]ScannedLabel, 0)
	if err = cursor.All(context.Background(), &scans); err != nil {
		log.Printf("Error decoding scan history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode scan history"})
		return
	}

	c.JSON(http.StatusOK, scans)
}

func handleCreateDailyLog(c *gin.Context) {
	uid, exists := c.Get("UID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	var req SaveDailyLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload format"})
		return
	}

	newLog := DailyLogEntry{
		UserID:    uid.(string),
		FoodName:  req.FoodName,
		Quantity:  req.Quantity,
		Unit:      req.Unit,
		Calories:  req.Calories,
		Protein:   req.Protein,
		Carbs:     req.Carbs,
		CreatedAt: time.Now(),
	}

	_, err := dailyLogCollection.InsertOne(context.Background(), newLog)
	if err != nil {
		log.Printf("Failed to save to MongoDB: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save daily log to database"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Daily log saved successfully!"})
}

func handleGetDailyLogs(c *gin.Context) {
	uid, exists := c.Get("UID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	filter := bson.M{"user_id": uid.(string)}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})

	cursor, err := dailyLogCollection.Find(context.Background(), filter, opts)
	if err != nil {
		log.Printf("Error fetching daily logs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch daily logs"})
		return
	}
	defer cursor.Close(context.Background())

	var logs []DailyLogEntry = make([]DailyLogEntry, 0)
	if err = cursor.All(context.Background(), &logs); err != nil {
		log.Printf("Error decoding daily logs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to decode daily logs"})
		return
	}

	c.JSON(http.StatusOK, logs)
}
