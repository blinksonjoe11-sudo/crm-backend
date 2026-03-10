package service

import (
	"context"
	"fmt"
	"mime/multipart"

	"github.com/google/uuid"
	cloudinaryutil "github.com/yourorg/crm/pkg/cloudinary"
)

type uploadService struct {
	client *cloudinaryutil.Client
}

func NewUploadService(client *cloudinaryutil.Client) UploadService {
	return &uploadService{client: client}
}

// folder returns the Cloudinary folder path: crm/{workspace_id}/{type}
func (s *uploadService) folder(wsID uuid.UUID, resourceType string) string {
	return fmt.Sprintf("crm/%s/%s", wsID.String(), resourceType)
}

func (s *uploadService) UploadContactAvatar(ctx context.Context, wsID uuid.UUID, contactID string, file interface{}, header interface{}) (string, error) {
	f, h, err := assertMultipart(file, header)
	if err != nil {
		return "", err
	}
	result, err := s.client.UploadFile(ctx, f, h, s.folder(wsID, "avatars"))
	if err != nil {
		return "", err
	}
	// Return optimised transformation URL for a 200x200 face crop
	return s.client.TransformURL(result.PublicID, "w_200,h_200,c_fill,g_face,f_auto,q_auto"), nil
}

func (s *uploadService) UploadCompanyLogo(ctx context.Context, wsID uuid.UUID, companyID string, file interface{}, header interface{}) (string, error) {
	f, h, err := assertMultipart(file, header)
	if err != nil {
		return "", err
	}
	result, err := s.client.UploadFile(ctx, f, h, s.folder(wsID, "logos"))
	if err != nil {
		return "", err
	}
	return s.client.TransformURL(result.PublicID, "w_300,h_300,c_pad,b_white,f_auto,q_auto"), nil
}

func (s *uploadService) UploadAttachment(ctx context.Context, wsID uuid.UUID, entityID string, file interface{}, header interface{}) (string, error) {
	f, h, err := assertMultipart(file, header)
	if err != nil {
		return "", err
	}
	result, err := s.client.UploadFile(ctx, f, h, s.folder(wsID, "attachments"))
	if err != nil {
		return "", err
	}
	return result.SecureURL, nil
}

func (s *uploadService) UploadWorkspaceLogo(ctx context.Context, wsID uuid.UUID, file interface{}, header interface{}) (string, error) {
	f, h, err := assertMultipart(file, header)
	if err != nil {
		return "", err
	}
	result, err := s.client.UploadFile(ctx, f, h, s.folder(wsID, "workspace"))
	if err != nil {
		return "", err
	}
	return s.client.TransformURL(result.PublicID, "w_400,h_400,c_pad,b_white,f_auto,q_auto"), nil
}

func assertMultipart(file, header interface{}) (multipart.File, *multipart.FileHeader, error) {
	f, ok1 := file.(multipart.File)
	h, ok2 := header.(*multipart.FileHeader)
	if !ok1 || !ok2 {
		return nil, nil, fmt.Errorf("invalid multipart file")
	}
	return f, h, nil
}
