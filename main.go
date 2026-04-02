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

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
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
		openai.WithModel("meta-llama/llama-4-scout-17b-16e-instruct"), // Target the Groq Vision model
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

	Use the following JSON structure if the image follows 1-4 instructions:
	{
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
		"score": "Score out of 100",
		"sentence": "A sentence regarding the health score"
		}
	}
	
	Use the following JSON structure if the image in under 6th and 7th instruction:
	{
	"error": "error message"	
	}`

	// 3. Create a Multimodal Message (Text + Image URL Data URI)
	// We pass the Base64 string using the standard data URI format required by OpenAI-compatible endpoints
	imageURI := fmt.Sprintf("data:image/jpeg;base64,%s", base64Image)

	message :=[]llms.MessageContent{
		{
			Role: llms.ChatMessageTypeHuman,
			Parts:[]llms.ContentPart{
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