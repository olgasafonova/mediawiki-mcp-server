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

	pages, err := c.queryPages(ctx, params)
	if err != nil {
		return GetImagesResult{}, err
	}

	imageTitles := imageTitlesFromPages(pages)
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
		return basicImagesResult(normalizedTitle, imageTitles), nil
	}

	return GetImagesResult{
		Title:  normalizedTitle,
		Images: images,
		Count:  len(images),
	}, nil
}

// imageTitlesFromPages collects image titles from all page objects in a
// query response.
func imageTitlesFromPages(pages map[string]interface{}) []string {
	var titles []string
	for _, p := range pages {
		titles = append(titles, imageTitlesFromPage(p)...)
	}
	return titles
}

// imageTitlesFromPage extracts image titles from a single page object.
func imageTitlesFromPage(p interface{}) []string {
	page := getMap(p)
	if page == nil {
		return nil
	}
	var titles []string
	for _, img := range getSlice(page["images"]) {
		if m := getMap(img); m != nil {
			titles = append(titles, getString(m["title"]))
		}
	}
	return titles
}

// basicImagesResult builds a title-only result used when the imageinfo
// lookup fails.
func basicImagesResult(normalizedTitle string, imageTitles []string) GetImagesResult {
	basicImages := make([]ImageInfo, 0, len(imageTitles))
	for _, t := range imageTitles {
		basicImages = append(basicImages, ImageInfo{Title: t})
	}
	return GetImagesResult{
		Title:  normalizedTitle,
		Images: basicImages,
		Count:  len(basicImages),
	}
}

// getImageInfo retrieves detailed info for images
func (c *Client) getImageInfo(ctx context.Context, titles []string) ([]ImageInfo, error) {
	if len(titles) == 0 {
		return nil, nil
	}

	// Batch in groups of 50
	var allImages []ImageInfo
	for _, batch := range chunkStrings(titles, 50) {
		allImages = append(allImages, c.imageInfoBatch(ctx, batch)...)
	}
	return allImages, nil
}

// imageInfoBatch fetches imageinfo for one batch of titles. Failed batches
// are skipped and yield no entries.
func (c *Client) imageInfoBatch(ctx context.Context, batch []string) []ImageInfo {
	params := url.Values{}
	params.Set("action", "query")
	params.Set("titles", strings.Join(batch, "|"))
	params.Set("prop", "imageinfo")
	params.Set("iiprop", "url|size|mime")
	params.Set("iiurlwidth", "300") // Get thumbnail URL

	pages, err := c.queryPages(ctx, params)
	if err != nil {
		return nil
	}

	images := make([]ImageInfo, 0, len(pages))
	for _, p := range pages {
		if page := getMap(p); page != nil {
			images = append(images, imageInfoFromPage(page))
		}
	}
	return images
}

// imageInfoFromPage builds an ImageInfo from a single page object, filling
// URL and dimension fields when imageinfo data is present.
func imageInfoFromPage(page map[string]interface{}) ImageInfo {
	imgInfo := ImageInfo{Title: getString(page["title"])}
	imageinfo := getSlice(page["imageinfo"])
	if len(imageinfo) == 0 {
		return imgInfo
	}
	info := getMap(imageinfo[0])
	if info == nil {
		return imgInfo
	}
	imgInfo.URL = getString(info["url"])
	imgInfo.ThumbURL = getString(info["thumburl"])
	imgInfo.Width = getInt(info["width"])
	imgInfo.Height = getInt(info["height"])
	imgInfo.Size = getInt(info["size"])
	imgInfo.MimeType = getString(info["mime"])
	return imgInfo
}
