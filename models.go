package main

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

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
	Calories         string                 `json:"calories,omitempty"`
	Protein          string                 `json:"protein,omitempty"`
	Carbs            string                 `json:"carbs,omitempty"`
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
