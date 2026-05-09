package wiki

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// GetImages retrieves images/files used on a page
func (c *Client) GetImages(ctx context.Context, args GetImagesArgs) (GetImagesResult, error) {
	if args.Title == "" {
		return GetImagesResult{}, fmt.Errorf("title is required")
	}

	if err := c.EnsureLoggedIn(ctx); err != nil {
		return GetImagesResult{}, err
	}

	limit := normalizeLimit(args.Limit, 50, MaxLimit)
	normalizedTitle := normalizePageTitle(args.Title)

	// First get list of images on the page
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", normalizedTitle)
	params.Set("prop", "images")
	params.Set("imlimit", strconv.Itoa(limit))

	resp, err := c.apiRequest(ctx, params)
	if err != nil {
		return GetImagesResult{}, err
	}

	query, ok := resp["query"].(map[string]interface{})
	if !ok {
		return GetImagesResult{}, fmt.Errorf("unexpected API response: missing 'query' object")
	}
	pages, ok := query["pages"].(map[string]interface{})
	if !ok {
		return GetImagesResult{}, fmt.Errorf("unexpected API response: missing 'pages' object")
	}

	var imageTitles []string
	for _, p := range pages {
		page, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if images, ok := page["images"].([]interface{}); ok {
			for _, img := range images {
				i, ok := img.(map[string]interface{})
				if !ok {
					continue
				}
				imageTitles = append(imageTitles, getString(i["title"]))
			}
		}
	}

	if len(imageTitles) == 0 {
		return GetImagesResult{
			Title:  normalizedTitle,
			Images: []ImageInfo{},
			Count:  0,
		}, nil
	}

	// Get image info (URLs, dimensions) for each image
	images, err := c.getImageInfo(ctx, imageTitles)
	if err != nil {
		// Return basic info without URLs if imageinfo fails
		basicImages := make([]ImageInfo, 0, len(imageTitles))
		for _, t := range imageTitles {
			basicImages = append(basicImages, ImageInfo{Title: t})
		}
		return GetImagesResult{
			Title:  normalizedTitle,
			Images: basicImages,
			Count:  len(basicImages),
		}, nil
	}

	return GetImagesResult{
		Title:  normalizedTitle,
		Images: images,
		Count:  len(images),
	}, nil
}

// getImageInfo retrieves detailed info for images
func (c *Client) getImageInfo(ctx context.Context, titles []string) ([]ImageInfo, error) {
	if len(titles) == 0 {
		return nil, nil
	}

	// Batch in groups of 50
	batchSize := 50
	var allImages []ImageInfo

	for i := 0; i < len(titles); i += batchSize {
		end := i + batchSize
		if end > len(titles) {
			end = len(titles)
		}
		batch := titles[i:end]

		params := url.Values{}
		params.Set("action", "query")
		params.Set("titles", strings.Join(batch, "|"))
		params.Set("prop", "imageinfo")
		params.Set("iiprop", "url|size|mime")
		params.Set("iiurlwidth", "300") // Get thumbnail URL

		resp, err := c.apiRequest(ctx, params)
		if err != nil {
			continue
		}

		query, ok := resp["query"].(map[string]interface{})
		if !ok {
			continue
		}
		pages, ok := query["pages"].(map[string]interface{})
		if !ok {
			continue
		}

		for _, p := range pages {
			page, ok := p.(map[string]interface{})
			if !ok {
				continue
			}
			title := getString(page["title"])

			imgInfo := ImageInfo{Title: title}

			if imageinfo, ok := page["imageinfo"].([]interface{}); ok && len(imageinfo) > 0 {
				info, ok := imageinfo[0].(map[string]interface{})
				if !ok {
					allImages = append(allImages, imgInfo)
					continue
				}
				imgInfo.URL = getString(info["url"])
				imgInfo.ThumbURL = getString(info["thumburl"])
				imgInfo.Width = getInt(info["width"])
				imgInfo.Height = getInt(info["height"])
				imgInfo.Size = getInt(info["size"])
				imgInfo.MimeType = getString(info["mime"])
			}

			allImages = append(allImages, imgInfo)
		}
	}

	return allImages, nil
}
