package cloudinaryutil

import (
	"context"
	"fmt"
	"mime/multipart"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

type ResourceType string

const (
	ResourceImage ResourceType = "image"
	ResourceVideo ResourceType = "video"
	ResourceRaw   ResourceType = "raw"
)

type UploadResult struct {
	PublicID     string    `json:"public_id"`
	URL          string    `json:"url"`
	SecureURL    string    `json:"secure_url"`
	Format       string    `json:"format"`
	Width        int       `json:"width,omitempty"`
	Height       int       `json:"height,omitempty"`
	Bytes        int       `json:"bytes"`
	ResourceType string    `json:"resource_type"`
	CreatedAt    time.Time `json:"created_at"`
}

type Client struct {
	cld       *cloudinary.Cloudinary
	cloudName string
}

func New(cloudName, apiKey, apiSecret string) (*Client, error) {
	cld, err := cloudinary.NewFromParams(cloudName, apiKey, apiSecret)
	if err != nil {
		return nil, fmt.Errorf("cloudinary: init failed: %w", err)
	}
	return &Client{cld: cld, cloudName: cloudName}, nil
}


func (c *Client) UploadFile(
	ctx context.Context,
	file multipart.File,
	header *multipart.FileHeader,
	folder string,
) (*UploadResult, error) {
	if header.Size > 25<<20 { 
		return nil, fmt.Errorf("cloudinary: file exceeds 25 MB limit")
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	resType := detectResourceType(ext)

	params := uploader.UploadParams{
		Folder:         folder,
		ResourceType:   string(resType),
		PublicID:       sanitiseFilename(header.Filename),
		Tags:           api.CldAPIArray{folder},
		Overwrite:      api.Bool(false),
		UniqueFilename: api.Bool(true),
	}

	// Auto-generate a square thumbnail for images via eager transformation
	if resType == ResourceImage {
		params.Eager = "w_400,h_400,c_fill,g_face,q_auto,f_auto"
	}

	result, err := c.cld.Upload.Upload(ctx, file, params)
	if err != nil {
		return nil, fmt.Errorf("cloudinary: upload failed: %w", err)
	}

	return &UploadResult{
		PublicID:     result.PublicID,
		URL:          result.URL,
		SecureURL:    result.SecureURL,
		Format:       result.Format,
		Width:        result.Width,
		Height:       result.Height,
		Bytes:        result.Bytes,
		ResourceType: result.ResourceType,
		CreatedAt:    result.CreatedAt,
	}, nil
}

// UploadURL fetches a remote URL and stores it in Cloudinary (e.g. Google avatar).
func (c *Client) UploadURL(ctx context.Context, remoteURL, folder, publicID string) (*UploadResult, error) {
	result, err := c.cld.Upload.Upload(ctx, remoteURL, uploader.UploadParams{
		Folder:       folder,
		PublicID:     publicID,
		ResourceType: "image",
		Overwrite:    api.Bool(true),
	})
	if err != nil {
		return nil, fmt.Errorf("cloudinary: url upload failed: %w", err)
	}
	return &UploadResult{
		PublicID:  result.PublicID,
		SecureURL: result.SecureURL,
		Format:    result.Format,
		Bytes:     result.Bytes,
	}, nil
}

// Delete removes an asset from Cloudinary by public ID.
func (c *Client) Delete(ctx context.Context, publicID string, resType ResourceType) error {
	_, err := c.cld.Upload.Destroy(ctx, uploader.DestroyParams{
		PublicID:     publicID,
		ResourceType: string(resType),
	})
	if err != nil {
		return fmt.Errorf("cloudinary: delete failed: %w", err)
	}
	return nil
}

// TransformURL builds a Cloudinary on-the-fly transformation URL.
// transformation example: "w_200,h_200,c_fill,f_auto,q_auto"
func (c *Client) TransformURL(publicID, transformation string) string {
	return fmt.Sprintf(
		"https://res.cloudinary.com/%s/image/upload/%s/%s",
		c.cloudName, transformation, publicID,
	)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func detectResourceType(ext string) ResourceType {
	images := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".gif": true, ".webp": true, ".svg": true, ".avif": true,
	}
	videos := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true,
		".mkv": true, ".webm": true,
	}
	if images[ext] {
		return ResourceImage
	}
	if videos[ext] {
		return ResourceVideo
	}
	return ResourceRaw
}

func sanitiseFilename(name string) string {
	name = strings.TrimSuffix(name, filepath.Ext(name))
	r := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ".", "_")
	name = r.Replace(name)
	if len(name) > 80 {
		name = name[:80]
	}
	return strings.ToLower(name)
}
