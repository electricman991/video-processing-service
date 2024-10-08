package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/joho/godotenv"
	ffmpeg "github.com/u2takey/ffmpeg-go"
)

var (
	rawVideoBucketName       string
	processedVideoBucketName string
	localRawVideoPath        string = "./raw-videos"
	localProcessedVideoPath  string = "./processed-videos"
)

/**
 * Creates the local directories for raw and processed videos.
 */

func setupDirectories() error {

	dev_err := godotenv.Load(".env")

	if dev_err != nil {
		fmt.Println("No .env file found. Using environemnt variables defined in production.")
	}

	rawVideoBucketName = os.Getenv("RAW_VIDEO_BUCKET_NAME")
	processedVideoBucketName = os.Getenv("PROCESSED_VIDEO_BUCKET_NAME")

	if err := ensureDirectoryExistence(localRawVideoPath); err != nil {
		return err
	}

	if err := ensureDirectoryExistence(localProcessedVideoPath); err != nil {
		return err
	}
	return nil
}

/**
 * @param rawVideoName - The name of the file to convert from {@link localRawVideoPath}.
 * @param processedVideoName - The name of the file to convert to {@link localProcessedVideoPath}.
 * @returns An error if failed or nil on successful video conversion
 */

func convertVideo(rawVideoName, processedVideoName string) error {
	// Construct the FFmpeg command to scale the video to 360p.
	err := ffmpeg.Input(fmt.Sprintf("%s/%s", localRawVideoPath, rawVideoName)).
		Output(fmt.Sprintf("%s/%s", localProcessedVideoPath, processedVideoName), ffmpeg.KwArgs{"vf": "scale=-1:360"}).
		OverWriteOutput(). // Ensure the output file is overwritten if it exists.
		ErrorToStdOut().   // Redirect FFmpeg's error output to stdout for logging.
		Run()              // Run the FFmpeg command.

	if err != nil {
		// If there is an error, log it and return the error.
		log.Printf("An error occurred: %v\n", err)
		return err
	}

	// Log a success message once the process is completed.
	log.Println("Processing finished successfully")
	return nil
}

/**
 * @param fileName - The name of the file to download from the
 * {@link rawVideoBucketName} bucket into the {@link localRawVideoPath} folder.
 * @returns An error if the file could not be downloaded or nil if successful
 */

func downloadRawVideo(fileName string, downloadChan chan error) {
	// sess := session.Must(session.NewSession())
	downloader := s3manager.NewDownloader(sess)

	f, err := os.Create(localRawVideoPath + "/" + fileName)
	if err != nil {
		downloadChan <- fmt.Errorf("failed to create file %q, %v", fileName, err)
		return
		// return fmt.Errorf("failed to create file %q, %v", fileName, err)
	}

	n, err := downloader.Download(f, &s3.GetObjectInput{
		Bucket: aws.String(rawVideoBucketName),
		Key:    aws.String(fileName),
	})
	if err != nil {
		downloadChan <- fmt.Errorf("failed to download file, %v", err)

		// Database cleanup when file does not exist
		query := "DELETE from yt_web_client_videos where id = ?"
		db.Exec(query, strings.Split(fileName, ".")[0])

		// Delete raw video file that was created
		deleteRawVideo(fileName)
		return
		// return fmt.Errorf("failed to download file, %v", err)
	}
	fmt.Printf("file downloaded, %d bytes\n", n)

	fmt.Printf("%s%s downloaded to ./raw-videos/%s\n", os.Getenv("SPACES_RAW_URL"), fileName, fileName)
	downloadChan <- nil
	// return nil
}

/**
 * @param fileName - The name of the file to upload from the
 * {@link localProcessedVideoPath} folder into the {@link processedVideoBucketName}.
 * @returns An error if failed or nil is successful
 */

func uploadProcessedVideo(fileName string, uploadChan chan error) {
	// The session the S3 Uploader will use
	// sess := session.Must(session.NewSession())

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	f, err := os.Open(localProcessedVideoPath + "/" + fileName)
	if err != nil {
		uploadChan <- fmt.Errorf("failed to open file %q, %v", fileName, err)
		// return fmt.Errorf("failed to open file %q, %v", fileName, err)
	}

	//Upload the file to S3.
	result, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(processedVideoBucketName),
		Key:    aws.String(fileName),
		Body:   f,
		ACL:    aws.String("public-read"),
	})
	if err != nil {
		uploadChan <- fmt.Errorf("failed to upload file, %v", err)
		// return fmt.Errorf("failed to upload file, %v", err)
	}
	fmt.Printf("file uploaded to, %s\n", aws.StringValue(&result.Location))
	uploadChan <- nil
	// return nil
}

/**
 * @param fileName - The name of the file to delete from the
 * {@link localRawVideoPath} folder.
 * @returns An error message on fail or nil for success
 *
 */

func deleteRawVideo(fileName string) error {
	err := os.Remove(localRawVideoPath + "/" + fileName)
	if err != nil {
		return err
	}
	fmt.Printf("File deleted at ./raw-videos/%s\n", fileName)
	return nil
}

/**
* @param fileName - The name of the file to delete from the
* {@link localProcessedVideoPath} folder.
* @returns An error message on fail or nil for success
*
 */
func deleteProcessedVideo(fileName string) error {
	err := os.Remove(localProcessedVideoPath + "/" + fileName)
	if err != nil {
		return err
	}
	fmt.Printf("File deleted at ./processed-videos/%s\n", fileName)
	return nil
}

/**
 * Ensures a directory exists, creating it if necessary.
 * @param {string} dirPath - The directory path to check.
 */
func ensureDirectoryExistence(dirPath string) error {
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return err
	}
	fmt.Printf("Directory created at %s\n", dirPath)
	return nil
}
