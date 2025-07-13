package article

import (
	"context"

	"meta-api/common/types"
)

// UserGetArticleList 获取文章列表
func (a *articleService) UserGetArticleList(ctx context.Context,
	request *types.UserGetArticleListRequest) (*types.UserGetArticleListResponse, error) {

	response := &types.UserGetArticleListResponse{}

	return response, nil
}

// UserGetArticleDetail 获取文章详情
func (a *articleService) UserGetArticleDetail(ctx context.Context,
	request *types.UserGetArticleDetailRequest) (*types.UserGetArticleDetailResponse, error) {

	response := &types.UserGetArticleDetailResponse{}

	return response, nil
}

// UserSearchArticle 搜索文章
func (a *articleService) UserSearchArticle(ctx context.Context,
	request *types.UserSearchArticleRequest) (*types.UserSearchArticleResponse, error) {

	response := &types.UserSearchArticleResponse{}

	return response, nil
}

// UserGetHotArticle 获取热门文章
func (a *articleService) UserGetHotArticle(ctx context.Context) (*types.UserGetHotArticleResponse, error) {
	response := &types.UserGetHotArticleResponse{}

	return response, nil
}

// UserGetTimeline 获取文章归档
func (a *articleService) UserGetTimeline(ctx context.Context) (*types.GetTimelineResponse, error) {
	response := &types.GetTimelineResponse{}

	return response, nil
}
