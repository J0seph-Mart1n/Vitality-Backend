package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
)

// Global variable to hold the Firebase Auth Client
var firebaseAuth *auth.Client
var mongoClient *mongo.Client
var labelCollection *mongo.Collection
var dailyLogCollection *mongo.Collection

// Struct for saving to MongoDB
type ScannedLabel struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	UserID    string                 `bson:"user_id" json:"user_id"`
	Data      map[string]interface{} `bson:"data" json:"data"`
	CreatedAt time.Time              `bson:"created_at" json:"created_at"`
}

// Struct for receiving the save request from React Native
type SaveScanRequest struct {
	Data map[string]interface{} `json:"data" binding:"required"`
}

type EstimateNutritionRequest struct {
	FoodName         string                 `json:"food_name" binding:"required"`
	NutritionalFacts map[string]interface{} `json:"nutritional_facts,omitempty"`
	ConsumedAmount   string                 `json:"consumed_amount" binding:"required"`
}

type DailyLogEntry struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    string             `bson:"user_id" json:"user_id"`
	FoodName  string             `bson:"food_name" json:"food_name"`
	Quantity  string             `bson:"quantity" json:"quantity"`
	Unit      string             `bson:"unit" json:"unit"`
	Calories  string             `bson:"calories" json:"calories"`
	Protein   string             `bson:"protein" json:"protein"`
	Carbs     string             `bson:"carbs" json:"carbs"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

type SaveDailyLogRequest struct {
	FoodName string `json:"food_name" binding:"required"`
	Quantity string `json:"quantity" binding:"required"`
	Unit     string `json:"unit" binding:"required"`
	Calories string `json:"calories" binding:"required"`
	Protein  string `json:"protein" binding:"required"`
	Carbs    string `json:"carbs" binding:"required"`
}

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("Warning: Error loading .env file or .env file not found")
	} else {
		fmt.Println(".env file loaded successfully!")
	}

	// Initialize Firebase Auth
	initFirebase()

	initMongo()

	r := gin.Default()

	r.POST("/analyze-label", AuthMiddleware(), handleImageUpload)
	r.POST("/estimate-nutrition", AuthMiddleware(), handleEstimateNutrition)

	r.POST("/save-scan", AuthMiddleware(), handleSaveScan)
	r.GET("/scan-history", AuthMiddleware(), handleGetScanHistory)
	
	r.POST("/daily-log", AuthMiddleware(), handleCreateDailyLog)
	r.GET("/daily-log", AuthMiddleware(), handleGetDailyLogs)

	fmt.Println("Server running on http://localhost:8080")
	r.Run(":8080")
}

func initFirebase() {
	// Provide the path to the service account JSON downloaded from Firebase Console
	opt := option.WithAuthCredentialsFile(option.ServiceAccount, "Service_account_json/vitality-cafab-firebase-adminsdk-fbsvc-ccea64d136.json")

	app, err := firebase.NewApp(context.Background(), nil, opt)
	if err != nil {
		log.Fatalf("Error initializing Firebase App: %v\n", err)
	}

	firebaseAuth, err = app.Auth(context.Background())
	if err != nil {
		log.Fatalf("Error getting Firebase Auth client: %v\n", err)
	}

	fmt.Println("Firebase initialized successfully!")
}

func initMongo() {
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		log.Fatal("MONGO_URI is not set in .env file")
	}

	clientOptions := options.Client().ApplyURI(mongoURI)
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}

	err = client.Ping(context.Background(), nil)
	if err != nil {
		log.Fatalf("Error pinging MongoDB: %v", err)
	}

	fmt.Println("Connected to MongoDB successfully!")
	mongoClient = client
	labelCollection = client.Database("scanHistory").Collection("labelScanReport")
	dailyLogCollection = client.Database("scanHistory").Collection("dailyLogs")
}

// AuthMiddleware intercepts incoming requests and verifies the Firebase ID Token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Get the Authorization header
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
			c.Abort()
			return
		}

		// 2. Extract the token from the "Bearer <token>" format
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format. Expected 'Bearer <token>'"})
			c.Abort()
			return
		}
		idToken := parts[1]

		// 3. Verify the token using Firebase Admin SDK
		token, err := firebaseAuth.VerifyIDToken(context.Background(), idToken)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token", "details": err.Error()})
			c.Abort()
			return
		}

		// 4. Token is valid! Set the user's UID in the context
		// This allows your handler to know exactly which user is making the request
		c.Set("UID", token.UID)

		// 5. Proceed to the actual handler (handleImageUpload)
		c.Next()
	}
}

// ---- Handlers ----

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

	analysisJSON, err := estimateNutritionWithGroq(req.FoodName, req.NutritionalFacts, req.ConsumedAmount)
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

// ---- Helper Functions ----

// encodeImageToBase64 reads the multipart file and returns a base64 string
func encodeImageToBase64(file io.Reader) (string, error) {
	bytes, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(bytes), nil
}

// analyzeImageWithGroq uses LangChainGo to hit Groq's Llama 4 Scout model
func analyzeImageWithGroq(base64Image string) (string, error) {
	ctx := context.Background()
	apiKey := os.Getenv("GROQ_API_KEY")

	// 1. Initialize the LangChainGo OpenAI client, but point it to Groq!
	llm, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL("https://api.groq.com/openai/v1"),
		openai.WithModel("meta-llama/llama-4-scout-17b-16e-instruct"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to init groq client: %v", err)
	}

	// 2. Define the Prompt Instructions
	promptText := `You are an expert nutritionist and health coach. 
	Look at the attached image of a food's nutritional label. 
	Analyze the ingredients and nutritional values, and provide the health benefits and harmful effects of consuming this product.

	Instructions:
	1. Read the image carefully to identify key ingredients and nutritional metrics (sugar, sodium, etc).
	2. List the benefits.
	3. List the harmful effects or warnings (e.g., high sugar, artificial preservatives).
	4. Give an insight (summary) about the food item in 3-4 sentences.
	5. Output your response STRICTLY as a valid JSON object. Do not use Markdown formatting blocks.
	6. If the image is not a nutritional label, return an JSON object {'error': 'Give an error message on why it is not a nutritional label'}.
	7. If the image is blurry or unreadable, return an JSON object {'error': 'Give an error message on why it is not readable'}.
	8. If you are able to provide more benefits and harmful effects of the food item, provide them using the proper JSON object format.
	9. If you are able to provide more or less nutritional facts than listed in the JSON object, provide them using the proper JSON object format.
	10. Provide a health score from 0-100 based on the nutritional facts and analysis made from the food label. Also provide a sentence regarding the health score.
	11. All the nutritional facts in the image label should be in the JSON object in the same format as the image label.
	12. Give a title of the food item.
	13. Find the Serving Size in the image label and provide it in the JSON object. The calories and other details displayed should be based on the serviing size.
	
	Use the following JSON structure if the image follows 1-4 instructions:
	{
	"title": "The title of the food item",
	"summary": "Short 1 sentence summary of the food item",
	"benefits": 
		{
		"benefit 1":"A senctence regarding the benefit", 
		"benefit 2":"A senctence regarding the benefit",
		"benefit 3":"A senctence regarding the benefit",
		},
	"harmful_effects": 
		{
		"harmful effect 1":"A senctence regarding the harmful effect", 
		"harmful effect 2":"A senctence regarding the harmful effect",
		"harmful effect 3":"A senctence regarding the harmful effect",
		},
	"nutritional_facts":
		{
		"Serving Size": "Amount in grams or ml based on the label"
		"calories":"Amount in kcal (Ex. 200 kcal)",
		"total_fat":"Amount in g (Ex. 10g)",
		"saturated_fat":"Amount in g (Ex. 5g)",
		"trans_fat":"Amount in g (Ex. 0g)",
		"cholesterol":"Amount in mg (Ex. 20mg)",
		"sodium":"Amount in mg (Ex. 100mg)",
		"total_carbohydrate":"Amount in g (Ex. 20g)",
		"dietary_fiber":"Amount in g (Ex. 5g)",
		"sugars":"Amount in g (Ex. 10g)",
		"protein":"Amount in g (Ex. 5g)",
		"vitamin_d":"Amount in mg (Ex. 20mg)",
		"calcium":"Amount in mg (Ex. 20mg)",
		"iron":"Amount in mg (Ex. 20mg)",
		"potassium":"Amount in mg (Ex. 20mg)",
		},
	"health_score": 
		{
		"score": "Score from 1-100",
		"sentence": "A sentence regarding the health score"
		}
	}
	
	Use the following JSON structure if the image in under 6th and 7th instruction:
	{
	"error": "error message"	
	}`

	// 3. Create a Multimodal Message (Text + Image URL Data URI)
	imageURI := fmt.Sprintf("data:image/jpeg;base64,%s", base64Image)

	message := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: promptText},
				llms.ImageURLContent{URL: imageURI},
			},
		},
	}

	// 4. Call the LLM
	resp, err := llm.GenerateContent(ctx, message)
	if err != nil {
		return "", fmt.Errorf("llm generation failed: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	// 5. Extract just the JSON object from the LLM response
	completion := resp.Choices[0].Content
	startIndex := strings.Index(completion, "{")
	endIndex := strings.LastIndex(completion, "}")
	if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
		cleanedJSON := completion[startIndex : endIndex+1]
		return cleanedJSON, nil
	}

	return "", fmt.Errorf("LLM did not return a valid JSON object: %s", completion)
}

// estimateNutritionWithGroq uses the same LLM but with a text-only prompt to estimate macros
func estimateNutritionWithGroq(foodName string, facts map[string]interface{}, consumedAmount string) (string, error) {
	ctx := context.Background()
	apiKey := os.Getenv("GROQ_API_KEY")

	// Reuse the exact same LLM client configuration
	llm, err := openai.New(
		openai.WithToken(apiKey),
		openai.WithBaseURL("https://api.groq.com/openai/v1"),
		openai.WithModel("meta-llama/llama-4-scout-17b-16e-instruct"),
	)
	if err != nil {
		return "", fmt.Errorf("failed to init groq client: %v", err)
	}

	var factsContext string
	if len(facts) > 0 {
		factsJSON, _ := json.MarshalIndent(facts, "", "  ")
		factsContext = fmt.Sprintf("\nHere are the exact nutritional facts from the product label:\n%s\nPlease base your estimation strictly on these facts (convert the serving accurately to match the consumed amount).\n", string(factsJSON))
	}

	promptText := fmt.Sprintf(`You are an expert nutritionist database. 
	I need the exact nutritional estimation strictly of serving %s for the following food/drink: "%s".%s
	Do not include any pleasantries or markdown formatting blocks.
	Output ONLY a raw, valid JSON object with the following structure:
	{
		"calories": "250",
		"protein": "12.5",
		"carbs": "33.2"
	}
	Ensure the values are strictly numbers represented as strings.`, consumedAmount, foodName, factsContext)

	message := []llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts: []llms.ContentPart{
				llms.TextContent{Text: promptText},
			},
		},
	}

	resp, err := llm.GenerateContent(ctx, message)
	if err != nil {
		return "", fmt.Errorf("llm generation failed: %v", err)
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("empty response from LLM")
	}

	completion := resp.Choices[0].Content
	startIndex := strings.Index(completion, "{")
	endIndex := strings.LastIndex(completion, "}")
	if startIndex != -1 && endIndex != -1 && endIndex > startIndex {
		cleanedJSON := completion[startIndex : endIndex+1]
		return cleanedJSON, nil
	}

	return "", fmt.Errorf("LLM did not return a valid JSON object: %s", completion)
}
