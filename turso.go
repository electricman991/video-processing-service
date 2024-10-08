package main

import (
	"database/sql"
	"fmt"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

type Status string

var (
	Processing Status = "processing"
	Processed  Status = "processed"
)

type Video struct {
	ID          *string
	UID         *string
	Filename    *string
	Status      *Status
	Title       *string
	Description *string
}

func getVideo(videoId string) (Video, error) {
	if db == nil {
		return Video{}, fmt.Errorf("database connection for getVideo is nil")
	}
	query := `
    SELECT id, uid, filename, status, title, description
    FROM yt_web_client_videos
    WHERE id = ?
    `

	row := db.QueryRow(query, videoId)

	// if errors != nil {
	// 	return Video{}, nil
	// }
	// defer rows.Close()

	var video Video
	var status string

	err := row.Scan(&video.ID, &video.UID, &video.Filename, &status, &video.Title, &video.Description)
	if err != nil {
		if err == sql.ErrNoRows {
			return Video{}, nil
		}
		return Video{}, err
	}

	if status != "" {
		videoStatus := Status(status)
		video.Status = &videoStatus
	}

	return video, nil
}

func setVideo(videoId string, video Video) error {
	query := `
    INSERT INTO yt_web_client_videos (id, uid, filename, status, title, description)
    VALUES (?, ?, ?, ?, ?, ?)
    ON CONFLICT(id) DO UPDATE SET
        uid = COALESCE(EXCLUDED.uid, yt_web_client_videos.uid),
        filename = COALESCE(EXCLUDED.filename, yt_web_client_videos.filename),
        status = COALESCE(EXCLUDED.status, yt_web_client_videos.status),
        title = COALESCE(EXCLUDED.title, yt_web_client_videos.title),
        description = COALESCE(EXCLUDED.description, yt_web_client_videos.description);
    `

	status := coalesceStatus(video.Status)

	stat, err := db.Prepare(query)
	if err != nil {
		return fmt.Errorf("prepare error %s", err)
	}
	// fmt.Printf(videoId, coalesceString(video.UID), coalesceStatus(video.Status), coalesceString(video.Title))
	_, err = stat.Exec(videoId, coalesceString(video.UID), coalesceString(video.Filename), status, coalesceString(video.Title), coalesceString(video.Description))
	return err

}

// Check if the value exists and if not provide a null value
func coalesceString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: *value, Valid: true}
}

// Check if the value exists and if not provide a null value
func coalesceStatus(value *Status) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: string(*value), Valid: true}
}

func isVideoNew(videoId string) (bool, error, bool) {
	if db == nil {
		return false, fmt.Errorf("database connection for isVideoNew is nil"), false
	}
	video, err := getVideo(videoId)
	if err != nil {
		return false, err, false
	}

	// check status
	if video.Status != nil {
		// The video is aalready processed
		if *video.Status == Processed {
			return false, nil, true
		}
	}

	return video.Status == nil, nil, false
}

func getVideos() ([]Video, error) {
	if db == nil {
		return []Video{}, fmt.Errorf("database connection for getVideos is nil")
	}
	query := `
    SELECT *
    FROM yt_web_client_videos
    LIMIT 10
    `

	rows, err := db.Query(query)

	if err != nil {
		return []Video{}, nil
	}
	defer rows.Close()

	var data []Video

	for rows.Next() {
		var video Video
		var status string
		errors := rows.Scan(&video.ID, &video.UID, &video.Filename, &status, &video.Title, &video.Description)

		if errors != nil {
			return []Video{}, errors
		}

		if status != "" {
			videoStatus := Status(status)
			video.Status = &videoStatus
		}

		data = append(data, video)
	}

	return data, nil
}
