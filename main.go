package main

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"

	"github.com/joho/godotenv"
)

// type VideoRequest struct {
// 	InputFilePath  string `json:"inputFilePath"`
// 	OutputFilePath string `json:"outputFilePath"`
// }

var (
	sess *session.Session
	db   *sql.DB
)

func init() {
	dev_err := godotenv.Load(".env")

	if dev_err != nil {
		fmt.Println("No .env file found")
	}

	var err error
	key := os.Getenv("SPACES_KEY")
	secret := os.Getenv("SPACES_SECRET")

	sess = session.Must(session.NewSession(&aws.Config{
		S3ForcePathStyle: aws.Bool(false),
		Region:           aws.String("us-east-1"),
		Endpoint:         aws.String("https://sfo3.digitaloceanspaces.com"),
		Credentials:      credentials.NewStaticCredentials(key, secret, ""),
	}))
	// s3Client = s3.New(sess)

	url := fmt.Sprintf("%s?authToken=%s", os.Getenv("TURSO_DATABASE_URL"), strings.ReplaceAll(os.Getenv("TURSO_AUTH_TOKEN"), "\n", ""))

	db, err = sql.Open("libsql", url)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open db %s: %s", url, err)
		os.Exit(1)
	}

	// db.SetConnMaxLifetime(5 * 60)

}

type PubSubMessage struct {
	Name string `json:"name"`
}

func processVideoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse the Pub/Sub message from the request body
	var pubSubData map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&pubSubData); err != nil {
		http.Error(w, fmt.Sprintf("json.NewDecoder: %v", err), http.StatusBadRequest)
		log.Printf("json.NewDecoder: %v", err)
		return
	}

	// Decode the base64-encoded message data
	dataBase64, ok := pubSubData["message"].(map[string]interface{})["data"].(string)
	if !ok {
		http.Error(w, "Invalid message payload received.", http.StatusBadRequest)
		log.Printf("Invalid message payload received.")
		return
	}

	dataBytes, err := base64.StdEncoding.DecodeString(dataBase64)
	if err != nil {
		http.Error(w, fmt.Sprintf("base64.DecodeString: %v", err), http.StatusBadRequest)
		log.Printf("base64.DecodeString: %v", err)
		return
	}

	// Parse the decoded JSON message
	var message PubSubMessage
	if err := json.Unmarshal(dataBytes, &message); err != nil {
		http.Error(w, fmt.Sprintf("json.Unmarshal: %v", err), http.StatusBadRequest)
		log.Printf("json.Unmarshal: %v", err)
		return
	}

	if message.Name == "" {
		http.Error(w, "Bad Request: missing filename.", http.StatusBadRequest)
		log.Printf("Bad Request: missing filename.")
		return
	}

	inputFileName := message.Name
	outputFileName := fmt.Sprintf("processed-%s", inputFileName)
	videoId := strings.Split(inputFileName, ".")[0]

	if isNew, err, isProcessed := isVideoNew(videoId); !isNew {
		if isProcessed {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		http.Error(w, "Bad Request: video already processing or processed.", http.StatusBadRequest)
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not retrieve the video from the database. Responded with: %s", err), http.StatusInternalServerError)
			return
		}
		return
	}

	if err := setVideo(videoId, Video{
		ID:     &videoId,
		UID:    StringPtr(strings.Split(videoId, "-")[0]),
		Status: &Processing,
	}); err != nil {
		http.Error(w, fmt.Sprintf("Could not save video metadata. Server responsed with: %s", err), http.StatusInternalServerError)
		return
	}

	// Download the raw video
	if err := downloadRawVideo(inputFileName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to download raw video. Responded with: %s", err), http.StatusInternalServerError)
		return
	}

	// Process the video
	if err := convertVideo(inputFileName, outputFileName); err != nil {
		deleteRawVideo(inputFileName)
		deleteProcessedVideo(outputFileName)
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		return
	}

	// Upload the processed video
	if err := uploadProcessedVideo(outputFileName); err != nil {
		if err := deleteRawVideo(inputFileName); err != nil {
			http.Error(w, fmt.Sprintf("Could not delete the raw video file: Error: %s", err), http.StatusInternalServerError)
			return
		}

		if err := deleteProcessedVideo(outputFileName); err != nil {
			http.Error(w, fmt.Sprintf("Could not delete the processed video file: Error: %s", err), http.StatusInternalServerError)
			return
		}
		http.Error(w, "Failed to upload processed video", http.StatusInternalServerError)
		return
	}

	setVideo(videoId, Video{
		Status:   &Processed,
		Filename: StringPtr(outputFileName),
	})

	// Clean up
	if err := deleteRawVideo(inputFileName); err != nil {
		http.Error(w, fmt.Sprintf("Could not delete the raw video file: Error: %s", err), http.StatusInternalServerError)
		return
	}

	if err := deleteProcessedVideo(outputFileName); err != nil {
		http.Error(w, fmt.Sprintf("Could not delete the processed video file: Error: %s", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Processing finished successfully"))
}

// Create a string pointer
func StringPtr(s string) *string {
	return &s
}

func main() {

	if err := setupDirectories(); err != nil {
		fmt.Printf("Failed initial directory setup server responded with %s", err)
	}

	http.HandleFunc("/process-video", processVideoHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server is running on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
