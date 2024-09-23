# Video Processing Service

The video processing service for the frontend client. This is a project to help showcase a fullstack application using NextJS with Golang handling the backend of processing the videos. See this [repo](https://github.com/electricman991/yt-web-client) for the frontend client.

## Running locally

To run the service locally you will need to have the following environment variables set:

```
SPACES_KEY
SPACES_SECRET
SPACES_BASE_URL
SPACES_RAW_URL
TURSO_DATABASE_URL
TURSO_AUTH_TOKEN
RAW_VIDEO_BUCKET_NAME
PROCESSED_VIDEO_BUCKET_NAME
```

Change the `.env.example` file to `.env` then add the values for each of these variables.

Then run:

```bash
go build -o video-processing-service
```
to build the binary and run it with: 
```bash
./video-processing-service
```

# Testing
Once the service is running you can test with any http client to send a request to `localhost:8081/process` with a body of `'{
      "message": {
        "data": "eyJuYW1lIjoiMzhmM2MzNzUtYjVjNi00N2RkLThhZmEtOTY4ODU3Mzk2ZmZkLTE3MjMyODA5ODY5OTMubXA0In0=",
        "messageId": "1234567890",
        "publishTime": "2024-08-03T12:34:56.789Z"
      },
    }'` where the `data` field is the base64 encoded json message and the `messageId` is a unique id for the message that will be used to track the status of the processing.

The base64 encoded json message can be generated with:
```bash
echo -n '{"name":"<filename>.mp4"}' | base64
```
where filename is the name of your video file.

Send the request with curl 
```bash
curl -X POST http://localhost:8081/process-video -H "Content-Type: application/json" -d '{
      "message": {
        "data": "eyJuYW1lIjoiMzhmM2MzNzUtYjVjNi00N2RkLThhZmEtOTY4ODU3Mzk2ZmZkLTE3MjI5MTE1OTg0MzEubXA0In0=",
        "messageId": "1234567890",
        "publishTime": "2024-08-03T12:34:56.789Z"
      },
    }'
```

This will send a request to the server and convert the video file uploaded by the user on the frontend into a processed video file that is stored in the cloud. The response from the server will be a message with the status of the processing.

## Build Docker Image

To build the docker image run:

```bash
docker build -t video-processing-service .
```

Deploy the docker image to a service such as Google Cloud Run or Digital Ocean App Platform or any other platform that supports containers. Set the environment 
variables in the cloud so that it will run properly.