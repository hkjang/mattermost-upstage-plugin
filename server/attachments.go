package main

import (
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mattermost/mattermost/server/public/model"
)

type botAttachment struct {
	FileID    string
	Name      string
	MIMEType  string
	Extension string
	Size      int64
	Content   []byte
}

func (p *Plugin) collectBotAttachments(fileIDs []string, channelID string) ([]botAttachment, error) {
	if len(fileIDs) == 0 {
		return nil, nil
	}

	attachments := make([]botAttachment, 0, len(fileIDs))
	for _, fileID := range fileIDs {
		fileID = strings.TrimSpace(fileID)
		if fileID == "" {
			continue
		}

		info, appErr := p.API.GetFileInfo(fileID)
		if appErr != nil {
			return nil, fmt.Errorf("failed to load Mattermost file info %q: %w", fileID, appErr)
		}
		if strings.TrimSpace(channelID) != "" && strings.TrimSpace(info.ChannelId) != "" && info.ChannelId != channelID {
			return nil, fmt.Errorf("Mattermost file %q does not belong to channel %q", attachmentLabel(info), channelID)
		}

		content, appErr := p.API.GetFile(fileID)
		if appErr != nil {
			return nil, fmt.Errorf("failed to download Mattermost file %q: %w", fileID, appErr)
		}

		attachments = append(attachments, botAttachment{
			FileID:    fileID,
			Name:      defaultIfEmpty(strings.TrimSpace(info.Name), fileID),
			MIMEType:  detectAttachmentMIMEType(info, content),
			Extension: strings.ToLower(strings.TrimPrefix(strings.TrimSpace(info.Extension), ".")),
			Size:      info.Size,
			Content:   content,
		})
	}

	return attachments, nil
}

func detectAttachmentMIMEType(info *model.FileInfo, content []byte) string {
	if info != nil {
		if value := strings.TrimSpace(info.MimeType); value != "" {
			return value
		}
		if value := strings.TrimSpace(info.Extension); value != "" {
			if detected := mime.TypeByExtension("." + strings.TrimPrefix(value, ".")); detected != "" {
				return detected
			}
		}
	}

	if len(content) > 0 {
		return http.DetectContentType(content)
	}

	return "application/octet-stream"
}

func attachmentLabel(info *model.FileInfo) string {
	if info == nil {
		return ""
	}
	if value := strings.TrimSpace(info.Name); value != "" {
		return value
	}
	return strings.TrimSpace(info.Id)
}

func sanitizeUploadFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "attachment"
	}
	base := filepath.Base(name)
	if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
		return "attachment"
	}
	return base
}
