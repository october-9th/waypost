package search

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Report là kết quả một lần search cùng những gì đã xảy ra trên đường đi.
type Report struct {
	// Results đã sắp xếp theo mode yêu cầu và cắt còn topN.
	Results []Result

	// Merged là toàn bộ kết quả đã gộp, giữ nguyên thứ tự relevance của nguồn.
	// Có mặt để đổi cách sắp xếp mà khỏi gọi lại API — lobste.rs là site nhỏ
	// tự host, đừng bắt nó phục vụ lại chỉ vì người dùng bấm đổi sort.
	Merged []Result

	// Repos là prior art trên GitHub: trả lời "đã có ai build chưa", không
	// phải "bài nào đáng đọc". Danh sách RIÊNG, không trộn vào Results — sao
	// GitHub và điểm HN không cùng thang, ép chung một bảng là bịa.
	Repos []Repo

	// LobstersTags là tag đã map từ topic. Rỗng nghĩa là topic không khớp tag
	// nào và lobste.rs bị bỏ qua — cần cho người dùng biết, đừng giấu.
	LobstersTags []string

	// Warnings là lỗi của từng nguồn khi nguồn còn lại vẫn chạy được.
	Warnings []error
}

// Search gọi cả ba nguồn song song và trả về top N.
//
// HN + lobste.rs trả lời "bài nào đáng đọc" và được gộp vào một bảng xếp hạng
// chung. GitHub trả lời một câu khác — "đã có ai build chưa" — nên nằm riêng
// ở Report.Repos.
//
// Một nguồn hỏng thì vẫn trả kết quả của nguồn kia kèm warning; chỉ báo lỗi
// khi cả HN lẫn lobste.rs cùng hỏng. Nửa kết quả vẫn hữu ích hơn là không có
// gì, nhưng người dùng phải biết mình đang xem kết quả thiếu.
func (c *Client) Search(ctx context.Context, topic string, topN int, mode SortMode) (Report, error) {
	topic = NormalizeTopic(topic)
	if topic == "" {
		return Report{}, errors.New("topic rỗng")
	}

	var (
		wg      sync.WaitGroup
		hnHits  []Result
		hnErr   error
		lobHits []Result
		lobTags []string
		lobErr  error
		repos   []Repo
		ghErr   error
	)

	wg.Add(3)
	go func() {
		defer wg.Done()
		hnHits, hnErr = c.SearchHN(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		lobHits, lobTags, lobErr = c.SearchLobsters(ctx, topic)
	}()
	go func() {
		defer wg.Done()
		repos, ghErr = c.SearchGitHub(ctx, topic)
	}()
	wg.Wait()

	if hnErr != nil && lobErr != nil {
		return Report{}, fmt.Errorf("cả hai nguồn đều hỏng: %w", errors.Join(hnErr, lobErr))
	}

	rep := Report{LobstersTags: lobTags, Repos: repos}
	// GitHub hỏng không bao giờ là lỗi chí mạng: nó trả lời câu hỏi phụ, và
	// rate limit 10 request/phút thì chạm là chuyện thường.
	for _, err := range []error{hnErr, lobErr, ghErr} {
		if err != nil {
			rep.Warnings = append(rep.Warnings, err)
		}
	}

	rep.Merged = Merge(hnHits, lobHits)
	rep.Results = SortResults(rep.Merged, mode)
	if topN > 0 && len(rep.Results) > topN {
		rep.Results = rep.Results[:topN]
	}
	return rep, nil
}
