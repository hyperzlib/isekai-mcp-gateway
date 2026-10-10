package wiki

import "context"

func (s *Service) resolvePageURL(ctx context.Context, c *Call, i Input) (Object, error) {
	v, err := c.Client.TitleInfo(ctx, i.URL)
	if err != nil {
		return nil, err
	}
	return Object{"title": v.Title, "page_id": v.PageID, "exists": v.Exists}, nil
}
