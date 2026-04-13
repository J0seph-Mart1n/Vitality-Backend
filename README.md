# Vitality-Backend - Go Lang Auth & LLM Server

This is the Go-based backend for the **Vitality** application. It serves as a secure bridge between the React Native frontend, Firebase Authentication, a local MongoDB instance, and the powerful Groq Llama-4-Scout LLM model.

## 🌟 Key Features

1. **Authentication Middleware:** 
   - Uses the Firebase Admin SDK to decode and verify JWT tokens sent by the frontend automatically via custom middleware (`AuthMiddleware`).
2. **Groq Llama 4 Integration:** 
   - Utilizes `LangChainGo` to construct multimodal prompts (Text + Base64 Image) and communicate seamlessly with Groq API endpoints.
   - Includes custom parsing logic to guarantee secure structured JSON responses from the LLM.
3. **Nutrition API Endpoints:** 
   - `POST /analyze-label`: Takes multipart images, converts them to base64, and prompts the LLM for strict nutritional label extraction.
   - `POST /estimate-nutrition`: Dynamically estimates unstructured "Daily Log" food portions by correlating macro facts against user-provided consumption strings.
4. **Data Persistence (MongoDB):** 
   - Direct integration using the standard `mongo-driver/mongo`.
   - Dedicated collections for `scan-history` and `daily-log`.
   - `GET` & `POST` CRUD operations isolated securely on a per-user (`UID` extracted from JWT) basis.

---

## 🚀 Setup & Installation

### Prerequisites
- Go (v1.20+)
- Local MongoDB running on standard port (`27017`)
- Firebase Service Account Key JSON File
- Groq API Key

---

### Step 1: Install Go Modules
In your terminal in the backend directory, run:

```bash
go mod tidy
```

### Step 2: Environment configuration (`.env`)
You need specific keys configured for the backend to function, including an absolute path linking to your Firebase Service Account JSON.
Create a `.env` file at the root of the backend folder `/Vitality-Backend` and populate it as follows:

```env
# Absolute path to your Firebase Admin SDK service account key json
GOOGLE_APPLICATION_CREDENTIALS=/absolute/path/to/Service_account_json/vitality-service-credential.json

# Your free Groq API key to power Llama 4 interaction
GROQ_API_KEY=gsk_your_groq_api_key_here

# Local or remote mongo connection string
MONGO_URI=mongodb://localhost:27017/
```

> **Note**: To obtain your Firebase Service Account JSON, go to your Firebase Project settings > Service Accounts > Generate new private key. Download this and place it securely on your disk, then point `GOOGLE_APPLICATION_CREDENTIALS` to its path.


### Step 3: Start the Go Server
Run the local Golang server instance:

```bash
go run .
```

The Gin routing server should output that it is successfully running locally on port `:8080`.

## 📂 Architecture Overview
- `main.go`: Application entrypoint, environment bootup, MongoDB/Firebase initialization, and Gin routing tree setup.
- `handlers.go`: HTTP endpoint controllers, request payload bindings, and MongoDB CRUD logic.
- `llm.go`: Specific logic focusing completely on Groq AI multimodal text/image prompt formulation and response JSON extraction using `LangChainGo`.
- `models.go`: Unified structs used for database interactions and Gin JSON binding targets.

## 📝 License
This project is for demonstration and personal use.
