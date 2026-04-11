package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/api/option"
)

// Global variable to hold the Firebase Auth Client
var firebaseAuth *auth.Client
var mongoClient *mongo.Client
var labelCollection *mongo.Collection
var dailyLogCollection *mongo.Collection

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
