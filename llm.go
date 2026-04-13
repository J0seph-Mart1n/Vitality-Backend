package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/openai"
)

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
func estimateNutritionWithGroq(foodName string, facts map[string]interface{}, consumedAmount string, calories string, protein string, carbs string) (string, error) {
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

	var userMacroContext string
	if calories != "" || protein != "" || carbs != "" {
		userMacroContext = "\nThe user has also provided the following known values. Give the same values as the user provided and adjust the values if not provided:"
		if calories != "" {
			userMacroContext += fmt.Sprintf(" Calories: %s.", calories)
		}
		if protein != "" {
			userMacroContext += fmt.Sprintf(" Protein: %sg.", protein)
		}
		if carbs != "" {
			userMacroContext += fmt.Sprintf(" Carbs: %sg.", carbs)
		}
		userMacroContext += "\n"
	}

	promptText := fmt.Sprintf(`You are an expert nutritionist database. 
	I need the exact nutritional estimation strictly of serving %s for the following food/drink: "%s".%s%s
	Do not include any pleasantries or markdown formatting blocks.
	Output ONLY a raw, valid JSON object with the following structure:
	{
		"calories": "250",
		"protein": "12.5",
		"carbs": "33.2"
	}
	Ensure the values are strictly numbers represented as strings.`, consumedAmount, foodName, factsContext, userMacroContext)

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
