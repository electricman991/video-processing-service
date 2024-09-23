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

var (
	sess *session.Session
	db   *sql.DB
)

func init() {
	dev_err := godotenv.Load(".env")

	if dev_err != nil {
		fmt.Println("No .env file found. Using environemnt variables defined in production.")
	}

	var err error
	key := os.Getenv("SPACES_KEY")
	secret := os.Getenv("SPACES_SECRET")
	spaces_base_url := os.Getenv("SPACES_BASE_URL")

	sess = session.Must(session.NewSession(&aws.Config{
		S3ForcePathStyle: aws.Bool(false),
		Region:           aws.String("us-east-1"),
		Endpoint:         aws.String(spaces_base_url),
		Credentials:      credentials.NewStaticCredentials(key, secret, ""),
	}))
	// s3Client = s3.New(sess)

	url := fmt.Sprintf("%s?authToken=%s", os.Getenv("TURSO_DATABASE_URL"), strings.ReplaceAll(os.Getenv("TURSO_AUTH_TOKEN"), "\n", ""))

	db, err = sql.Open("libsql", url)

	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open db %s: %s", url, err)
		os.Exit(1)
	}
}

type PubSubMessage struct {
	Name string `json:"name"`
}

func processVideoHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		log.Printf("Method not allowed: %s", r.Method)
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
			log.Println("Video already processed")
			return
		}
		http.Error(w, "Bad Request: video already processing or processed.", http.StatusBadRequest)
		log.Printf("Bad Request: video already processing or processed.")
		if err != nil {
			http.Error(w, fmt.Sprintf("Could not retrieve the video from the database. Responded with: %s", err), http.StatusInternalServerError)
			log.Printf("Could not retrieve the video from the database. Responded with: %s", err)
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
		log.Printf("Could not save video metadata. Server responsed with: %s", err)
		return
	}

	// Download the raw video
	downloadChan := make(chan error)

	go downloadRawVideo(inputFileName, downloadChan)
	result := <-downloadChan

	if result != nil {
		http.Error(w, fmt.Sprintf("Failed to download raw video. Responded with: %s", result), http.StatusInternalServerError)
		log.Printf("Failed to download raw video. Responded with: %s", result)
		return
	}

	// Process the video
	if err := convertVideo(inputFileName, outputFileName); err != nil {
		deleteRawVideo(inputFileName)
		deleteProcessedVideo(outputFileName)
		http.Error(w, "Processing failed", http.StatusInternalServerError)
		log.Printf("Processing failed: %s", err)
		return
	}

	// Upload the processed video
	uploadChan := make(chan error)

	go uploadProcessedVideo(outputFileName, uploadChan)
	uploadRes := <-uploadChan
	if uploadRes != nil {
		if err := deleteRawVideo(inputFileName); err != nil {
			http.Error(w, fmt.Sprintf("Could not delete the raw video file: Error: %s", err), http.StatusInternalServerError)
			log.Printf("Could not delete the raw video file: Error: %s", err)
			return
		}

		if err := deleteProcessedVideo(outputFileName); err != nil {
			http.Error(w, fmt.Sprintf("Could not delete the processed video file: Error: %s", err), http.StatusInternalServerError)
			log.Printf("Could not delete the processed video file: Error: %s", err)
			return
		}
		http.Error(w, "Failed to upload processed video", http.StatusInternalServerError)
		log.Printf("Failed to upload processed video: %s", err)
		return
	}

	setVideo(videoId, Video{
		Status:   &Processed,
		Filename: StringPtr(outputFileName),
	})

	// Clean up
	if err := deleteRawVideo(inputFileName); err != nil {
		http.Error(w, fmt.Sprintf("Could not delete the raw video file: Error: %s", err), http.StatusInternalServerError)
		log.Printf("Could not delete the raw video file: Error: %s", err)
		return
	}

	if err := deleteProcessedVideo(outputFileName); err != nil {
		http.Error(w, fmt.Sprintf("Could not delete the processed video file: Error: %s", err), http.StatusInternalServerError)
		log.Printf("Could not delete the processed video file: Error: %s", err)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Processing finished successfully"))
}

// func retrieveVideos(w http.ResponseWriter, r *http.Request) {
// 	data, err := getVideos()

// 	if err != nil {
// 		http.Error(w, fmt.Sprintf("Could not retieve data from database. Responded with error: %s", err), http.StatusBadRequest)
// 		return
// 	}

// 	w.Header().Set("Content-Type", "application/json")
// 	if err := json.NewEncoder(w).Encode(data); err != nil {
// 		http.Error(w, "Failed to encode data", http.StatusInternalServerError)
// 	}

// }

// Create a string pointer
func StringPtr(s string) *string {
	return &s
}

func main() {

	if err := setupDirectories(); err != nil {
		fmt.Printf("Failed initial directory setup server responded with %s", err)
	}

	http.HandleFunc("/process-video", processVideoHandler)

	// http.HandleFunc("/get-videos", retrieveVideos)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	fmt.Printf("Server is running on port %s\n", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
