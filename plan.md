# albert — Tech Article Finder — Implementation Plan (V1, revised)

Personal CLI tool: nhập topic → trả về bài đáng đọc từ Hacker News + lobste.rs, dùng **native score (upvotes) của cộng đồng** làm tín hiệu chất lượng thay vì tự chấm điểm bằng heuristic.

## Vì sao đổi hướng so với bản Brave + heuristic scoring

Bản trước cố tự đoán "bài nào hay" bằng keyword matching (title chứa "internals" → +0.08) và domain whitelist tự chế — về bản chất vẫn là đoán, không giải quyết đúng vấn đề gốc: **khó tự xác định bài nào hay trước khi đọc**.

HN và lobste.rs đã có sẵn tín hiệu chất lượng thật: điểm số do cộng đồng bỏ phiếu. Dùng trực tiếp signal đó thay vì tự chế công thức trọng số.

Medium bị loại khỏi V1 vì không có tín hiệu chất lượng nào đáng tin qua API (claps dễ bị game, không sort được theo "hay"). Nếu sau này cần mở rộng nguồn, follow RSS của publication Medium cụ thể bạn tự biết là tốt — không search Medium rộng.

## Stack

- **Language**: Go
- **Nguồn**: Hacker News (Algolia Search API) + lobste.rs (**JSON endpoint, không phải RSS** — xem mục 2)
- **Storage**: SQLite (cache kết quả, optional)
- Không cần: Brave API, YAML scoring config, query expansion, dedup URL phức tạp, rate limiter

## Kiến trúc pipeline

```
topic
  │
  ▼
HN Algolia Search API   (query = topic, tags=story)
  │
  ▼
lobste.rs /t/{tag}.json  (lọc theo tag; KHÔNG filter RSS newest)
  │
  ▼
Merge + dedup nhẹ         (cùng URL từ 2 nguồn → gộp lại)
  │
  ▼
Sort theo native score    (points của HN, score của lobste.rs)
  │
  ▼
Top N + CLI output
```

## 1. Hacker News — Algolia Search API

Free, không cần API key.

```
GET https://hn.algolia.com/api/v1/search?query={topic}&tags=story&hitsPerPage=30
```

Response trả về `points`, `title`, `url`, `created_at`, `num_comments` — dùng `points` trực tiếp làm quality signal, không cần scoring riêng.

Có thể thêm filter theo thời gian nếu muốn ưu tiên bài gần đây hơn (`numericFilters=created_at_i>...`), nhưng V1 chưa cần — để mặc định sort theo relevance/points của chính Algolia.

**Đã verify bằng call thật (2026-08-26):**

- Field trả về đúng như trên. Đầy đủ: `points, title, url, created_at, created_at_i, num_comments, objectID, author, story_id, children, _tags`.
- Query `"go scheduler"` → 653 hit, top 5 đều đúng chủ đề, điểm 255 → 13.
- **`url` có thể là `null`** (Ask HN / text post không có link ngoài). **Phải lọc bỏ**, nếu không sẽ ra dòng rỗng trong output.
- **Recall với topic niche mỏng** — đo thật: `sqlite wal internals` = 7 hit, `zig comptime allocator` = 6, `elixir beam scheduler` = **2**. Không phải zero, nhưng topic hẹp thì không đủ cho top 10. Đây là trade-off chính khi bỏ Brave: đổi recall lấy precision.

```go
type HNHit struct {
    Title     string `json:"title"`
    URL       string `json:"url"`
    Points    int    `json:"points"`
    CreatedAt string `json:"created_at"`
}

func searchHN(topic string) ([]HNHit, error) {
    endpoint := fmt.Sprintf(
        "https://hn.algolia.com/api/v1/search?query=%s&tags=story&hitsPerPage=30",
        url.QueryEscape(topic),
    )
    // GET request, parse JSON response vào []HNHit
}
```

## 2. lobste.rs — JSON endpoint

**Đã verify (2026-08-26) — dùng `.json`, KHÔNG dùng RSS.**

RSS **không có** field score. Toàn bộ tag trong `<item>`: `title, link, guid, author, pubDate, comments, description, category`. Không có gì để làm quality signal.

Endpoint `.json` có tồn tại và có `score` — cả hai đều trả HTTP 200:

```
https://lobste.rs/t/{tag}.json     → story theo tag
https://lobste.rs/hottest.json     → story đang hot
```

Field: `score, title, url, tags, comment_count, created_at, description, description_plain, short_id, short_id_url, comments_url, submitter_user, flags, user_is_author`.

Ưu điểm so với RSS: có `score`, có `tags` để lọc, và khỏi parse XML.

**Bỏ hẳn cách "lấy `/rss` newest rồi filter keyword theo title"** — cách đó lọc vài chục bài đăng trong 1-2 ngày qua theo một topic cụ thể, tỉ lệ khớp gần như bằng 0.

**Bước 0 đã verify (2026-08-26) — KHÔNG có search JSON, chốt đi theo tag map.**

`https://lobste.rs/search.json?q=...` → **HTTP 400** `{"error":"400 Unpermitted query or form parameter"}`. Suffix `.json` chỉ có trên index endpoint, không có trên `/search`.

Search chỉ có bản HTML và không dùng được cho pipeline này:

- `/search?q=go+scheduler&what=stories` → **1777 kết quả** = OR-matching, không phải AND.
- Ngoặc kép `"go scheduler"` → vẫn 1777 → **không hỗ trợ phrase search**.
- `order=score` top 5 cho "go scheduler": Zig 0.16 Release Notes (163), A Pipeline Made of Airbags (144), Writing Toy Software Is A Joy (135) — rác hoàn toàn, đúng như hệ quả của OR-matching.
- `order=relevance` khá hơn nhưng vẫn lệch (top 1 là bài Linux I/O scheduler 2017, 6 điểm). Vì ta sort lại theo score, scrape relevance rồi re-sort sẽ tái tạo đúng đống rác trên.
- Bẫy: thêm `utf8=%E2%9C%93` vào query string làm server **nuốt luôn param `q`** → 0 results. Đừng copy nguyên form field.

Đường tag verify sạch, đi hướng này:

- `/t/{tag}.json` → 200, array 25 story, đủ field score.
- Multi-tag: `/t/go,rust.json` → 200.
- Pagination: `/t/go/page/2.json` → 200. (Dạng `?page=2` bị bỏ qua, trả về trang 1.)
- `/tags.json` → 200, **116 tag** (114 active), mỗi entry có `tag` + `description` → dùng map topic → tag.
- Thứ tự trả về là **newest**, không phải top → phải tự sort theo `score`. Score thực tế trang 1 `/t/go`: 19, 6, 12, 16, 1, 11, 75, 2, 27, 2, 17, 36, 13, 84...

**Cách làm mục 2:**

1. Fetch `/tags.json` một lần, cache dài hạn (tag set gần như không đổi).
2. Match topic với `tag` + `description` → chọn 1-2 tag.
3. Lấy 2-3 page của tag đó (`/t/{tag}.json`, `/t/{tag}/page/2.json`).
4. Nếu topic hẹp hơn tag (vd "go scheduler" vs tag `go`), lọc thêm title theo keyword.
5. Không khớp tag nào → **bỏ qua lobste.rs, chạy HN-only**. Không fallback sang search.

```go
type LobstersItem struct {
    Title string
    URL   string
    Score int
}

func fetchLobsters(tag string, pages int) ([]LobstersItem, error) {
    // page 1: https://lobste.rs/t/{tag}.json
    // page n: https://lobste.rs/t/{tag}/page/{n}.json   (KHÔNG dùng ?page=n — bị bỏ qua)
    // Parse JSON, đọc thẳng field score. Trả về theo newest → tự sort sau.
    // Nhớ set User-Agent tử tế, đây là site nhỏ tự host.
}
```

## 3. Merge + dedup

Chỉ cần dedup nhẹ vì số lượng nguồn nhỏ (2 nguồn, không phải merge 100 kết quả từ 7 query variant như bản trước):

**Đã có sẵn code + test**: `internal/search/dedup.go` (viết cho bản Brave, dùng lại được nguyên vẹn). Bản đó nhỉnh hơn snippet dưới đây: xử lý thêm `fragment` (`#section`) và `www.` — hai nguồn trùng lặp hay gặp.

```go
func normalizeURL(raw string) string {
    u, err := url.Parse(raw)
    if err != nil {
        return raw
    }
    u.Scheme = "https"
    u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
    u.RawQuery = ""
    u.Fragment = ""
    u.Path = strings.TrimSuffix(u.Path, "/")
    return u.String()
}
```

Cùng URL xuất hiện ở cả HN và lobste.rs → gộp lại, giữ điểm cao hơn (vd `max(hn_points, lobsters_score)`), đánh dấu "xuất hiện trên cả 2" — bản thân đây cũng là 1 tín hiệu chất lượng nhẹ, không cần công thức phức tạp.

## 4. Sort & output

```go
type Result struct {
    Title  string
    URL    string
    Score  int
    Source string // "HN", "Lobsters", "HN+Lobsters"
}
```

Sort giảm dần theo `Score`, lấy top N (10-15). Không có bước "ranking" riêng — điểm cộng đồng chính là ranking.

```
albert search "go scheduler"

1. 342  Scheduling In Go: Part I           [HN]
2. 128  The Go scheduler internals          [HN+Lobsters]
3.  67  Understanding GOMAXPROCS            [Lobsters]
```

## 5. Cache (optional, nhẹ hơn nhiều so với bản trước)

SQLite table đơn giản để tránh gọi lại API khi search cùng topic trong thời gian ngắn (TTL vài ngày là đủ, vì HN/lobsters có kết quả khá ổn định theo topic — không như cần refresh liên tục):

```
search_cache(topic, results_json, fetched_at)
```

## Không làm ở V1

- Medium — để dành nếu cần mở rộng, làm qua RSS publication cụ thể, không search rộng.
- Brave Search fallback cho topic niche — chỉ thêm nếu thực tế thấy HN+lobsters không đủ kết quả cho nhiều topic bạn quan tâm.
- Bất kỳ heuristic/LLM scoring nào — native score đã đóng vai trò đó.
- Feedback loop save/useful — có thể thêm sau nếu muốn cá nhân hóa thêm (vd domain/author bạn hay thấy hữu ích), nhưng không phải requirement để ship V1.

## Thứ tự build

0. ~~Verify `https://lobste.rs/search.json?q=...`~~ — **XONG (2026-08-26): không có search JSON, chốt tag map.** Xem mục 2.
1. ~~HN Algolia client~~ — **XONG**, `internal/search/hn.go`. Lọc bỏ hit `url` null.
2. ~~lobste.rs JSON client~~ — **XONG**, `internal/search/lobsters.go`. `/tags.json` (cache 7 ngày) → map topic sang tag → `/t/{a},{b}.json` + `/page/{n}.json`, 3 trang.
3. ~~Merge/dedup 2 nguồn~~ — **XONG**, `internal/search/merge.go`. Trùng URL → max score, OR cờ Source.
4. ~~Sort theo score + CLI output~~ — **XONG**, `main.go`. `albert search [-n] [-timeout] [-json] "<topic>"`.
5. SQLite cache — **CHƯA LÀM, và đề nghị bỏ.** Xem bên dưới.
6. Dùng thử vài tuần — nếu thấy thiếu nguồn (topic niche không có trên HN/lobsters), mới cân nhắc thêm Medium RSS theo publication cụ thể hoặc Brave fallback.

## Phát sinh khi build (2026-08-26)

**Thêm bộ lọc precision cho HN.** Algolia cũng match kiểu OR: topic `elixir beam scheduler` trả về top 1 là bài Rust/Bevy 110 điểm — lạc đề hoàn toàn nhưng điểm cao nên leo lên đầu khi sort theo score. Nay bỏ hit mà cả title lẫn URL đều không nhắc token nào của topic. Đây là lọc precision, không phải chấm điểm — vẫn không tự đoán bài nào hay.

**Hai danh sách stopword, không phải một.** `genericTagWords` ("programming", "language", "design", "system") chỉ bỏ khi map topic sang tag; nếu bỏ luôn ở bước lọc title thì `compiler design` mất từ khóa "design" và feed tag `compilers` lọt bài lạc đề vào top 3. `grammarWords` bỏ ở mọi nơi.

**Về cache SQLite (mục 5):** đề nghị bỏ khỏi V1. Lý do: SQLite trong Go cần cgo (mattn) hoặc modernc — thêm dependency nặng vào một project đang zero-dep, để đổi lấy việc tiết kiệm 4 HTTP request cho một CLI chạy tay vài lần một ngày. Đang cache đúng thứ cần cache: `tags.json` của lobste.rs (TTL 7 ngày, file JSON ở `os.UserCacheDir()`). Nếu về sau thật sự search lại cùng topic nhiều thì thêm cache kết quả cũng bằng file JSON, vẫn không cần SQLite.

**Chưa có test cho đường lỗi mạng.** `Client` gọi URL hardcode nên không inject được test server. Logic thuần (tag map, stem, merge, dedup) đã có test. Nếu sau này thấy cần chắc chắn "một nguồn hỏng thì nguồn kia vẫn ra kết quả", phải cho base URL vào struct trước.

## Đo thật sau khi build xong (2026-08-26)

| Topic | Kết quả | lobste.rs đóng góp |
|---|---|---|
| `go scheduler` | 10 bài, top 1 = 255đ, toàn bài đúng chủ đề | 0 (tag `go`, không bài nào khớp "scheduler") |
| `rust borrow checker` | top 3 đều 173-333đ, đúng chủ đề | 0 |
| `sqlite wal internals` | 4 bài | **3/4** — nguồn chính, HN chỉ có 1 |
| `distributed consensus` | 5 bài 212-454đ | 1 bài HN+Lobsters (tín hiệu 2 nguồn hoạt động) |
| `compiler design` | top 3 đúng chủ đề | 0 |
| `elixir beam scheduler` | **0 bài** — HN chỉ có 2 hit, cả 2 lạc đề, bị lọc hết | 0 |
| `quantum knitting patterns` | 0 bài, báo rõ "không khớp tag nào" | không gọi |

Kết luận: HN gánh phần lớn. lobste.rs đáng giữ vì với topic HN mỏng (`sqlite wal internals`) nó lại là nguồn chính. Topic niche vẫn có thể ra 0 kết quả — đúng như trade-off đã lường trước khi bỏ Brave.

---

# V2 — TUI (chưa chốt, mở từ 2026-08-26)

V1 đã ship: `albert search "<topic>"` in ra top N rồi thoát. Vấn đề khi dùng thật hằng ngày là vòng lặp không khớp: **gõ topic → lướt kết quả → mở vài bài → đổi topic thử lại**. Chạy lại lệnh cho mỗi vòng thì mệt, nên cần TUI.

Mục tiêu: **dễ dùng hằng ngày**. Đây là tiêu chí số một, trên cả tính năng.

## Ràng buộc đã chốt

- **Native macOS binary, không container.** Đã cân nhắc Docker + OrbStack và bỏ: với Go single-binary thì container làm việc dùng hằng ngày khó hơn — mất shell completion, cache `os.UserCacheDir()` kẹt trong container, quoting qua `docker run` phiền.
- **`internal/search` giữ nguyên làm lõi.** TUI là frontend thứ hai bên cạnh CLI hiện có, không viết lại logic search vào tầng UI.
- Không quay lại heuristic/LLM scoring. Lý do ở phần đầu file này.

## Bốn câu hỏi — đã chốt (2026-08-26)

1. **Thư viện TUI: bubbletea.** Đây là dependency ngoài stdlib đầu tiên, và nó
   không đi một mình: `bubbles` (textinput, spinner), `lipgloss` (màu + đo bề
   rộng hiển thị), cộng ~20 module gián tiếp. `bubbles/textinput` còn kéo theo
   `atotto/clipboard` cho tính năng paste — có sẵn trong binary rồi nên phím
   `y` (copy link) dùng luôn nó thay vì tự shell ra `pbcopy`. Binary từ ~3MB
   lên ~10MB. Đổi lại: không phải tự viết vòng lặp phím, ô nhập, và phần đo bề
   rộng ký tự CJK.
2. **Đọc bài: mở browser, không có màn hình đọc trong terminal.** `enter` mở
   bài, `c` mở trang thảo luận (thread HN hoặc lobste.rs). Thread nhiều khi
   đáng đọc ngang bài gốc nên mới tách phím riêng.
3. **Không lưu trạng thái.** Không đánh dấu đã đọc, không để dành. Câu hỏi
   SQLite vẫn hoãn. Cache duy nhất vẫn là `tags.json` (file JSON, TTL 7 ngày).
4. **Cài đặt: `go install` hoặc `make build` rồi tự copy binary.**
   `make build` → `./bin/albert`; `make install` → `$(go env GOPATH)/bin`.

## Đã build (2026-08-26)

`internal/search` không đổi về logic — TUI là frontend thứ hai, không phải
bản viết lại. Phần thêm vào lõi chỉ là dữ liệu để hiển thị và để mở thread:

- `Result.CommentsURL` + `NumComments`: HN dựng từ `objectID`
  (`news.ycombinator.com/item?id=`), lobste.rs lấy `comments_url` +
  `comment_count`. Merge giữ nguyên cặp URL-và-số đếm của một nguồn, không
  trộn số của nguồn này với link của nguồn kia. Bài ở cả hai nguồn giữ thread
  HN vì caller truyền HN trước, và thread HN thường đông hơn.
- `Result.Description`: `description_plain` của lobste.rs. HN không có gì
  tương đương nên phần lớn kết quả để trống — hiển thị dưới dạng dòng phụ của
  bài đang chọn, không phải cột.

`internal/tui`:

- Một màn hình: ô nhập ở trên, danh sách ở giữa, link + mô tả của bài đang
  chọn ở dưới, footer phím tắt. Danh sách vẽ số dòng cố định và đệm dòng
  trống, để khung dưới không nhảy khi số kết quả thay đổi.
- Phím: `enter` mở bài · `c` mở thảo luận · `y` copy link · `j/k g G` chọn ·
  `/` topic mới (xoá sạch ô nhập) · `i` sửa topic đang gõ · `r` tìm lại ·
  `q` thoát.
- Có kết quả thì focus tự nhảy sang danh sách, `j/k` dùng được ngay.
- Mỗi lần tìm mang một số thứ tự; kết quả về trễ của lần tìm cũ bị bỏ. Lần
  tìm mới cũng huỷ context của lần cũ.
- Hẹp hơn 72 cột thì bỏ cột nguồn bên phải, đẩy nhãn nguồn xuống dòng phụ —
  không thì 11 ô đó ăn mất tiêu đề.
- `openURL` chỉ nhận `http`/`https`. Link đến từ API bên ngoài, mà `open` trên
  macOS thi hành cả scheme khác.

Vào lệnh:

```
albert                        # TUI rỗng
albert "go scheduler"         # TUI, tìm luôn
albert search "go scheduler"  # CLI cũ, không đổi
```

`albert` mà stdout không phải terminal thì báo lỗi và chỉ sang `search`,
thay vì để bubbletea chết giữa chừng.

## Thử thật bằng pty (2026-08-26)

TUI không test tay được trong môi trường không có terminal, nên có hai lớp:

- **Test đơn vị** (`internal/tui`): con trỏ + cửa sổ cuộn, kết quả cũ về trễ
  không ghi đè state mới, `View()` không tràn màn hình ở 5 kích thước, chặn
  scheme không phải web.
- **Chạy thật trong pty**: `scripts/tui-pty.py` fork pty, gõ phím thật, dựng
  lại màn hình từ chuỗi escape; kịch bản ở `scripts/scenarios/`.

  ```
  make build
  SCENARIO=scripts/scenarios/basic.py python3 scripts/tui-pty.py bin/albert 30 100
  ```

  Đã chạy `go scheduler` (15 kết quả, 11 hiện trên màn
  30 dòng), `sqlite wal internals` ở 60 cột (layout hẹp), và
  `quantum knitting patterns` (0 kết quả, báo đúng).

  **Bẫy khi tự dựng harness pty:** termenv hỏi màu nền bằng OSC 10/11 rồi
  `CSI 6n`, sau đó **đọc stdin chờ trả lời với timeout 5 giây mỗi query** —
  không trả lời thì nó nuốt sạch phím gõ trong lúc chờ, và triệu chứng là
  "app chạy nhưng không nhận phím". Harness phải đóng vai terminal mà trả lời
  hai query đó. Terminal thật trả lời tức thì nên không ai gặp.

Hai lỗi UX chỉ lộ ra khi chạy pty, đã sửa:

- `/` trước đây giữ nguyên chữ cũ nên topic mới bị nối vào topic cũ
  (`sqlite wal internalsquantum knitting patterns`) → nay `/` xoá sạch, `i`
  mới là sửa.
- Bấm `esc` giữa lúc gõ dở thì header còn chữ nửa vời trong khi danh sách vẫn
  là kết quả cũ → nay trả ô nhập về đúng topic của kết quả đang xem.

## Chưa làm ở V2

- Lưu bài đã đọc / để dành — xem câu 3, cố tình không làm.
- Đọc bài trong terminal — xem câu 2.
- Tự động test phím `enter`/`c`/`y`: mở browser thật và ghi đè clipboard thật,
  nên chỉ test phần chặn scheme. Phần gọi `open` vẫn phải thử tay.


---

# V3 — GitHub + sort mode (2026-08-26)

Sinh ra từ một câu hỏi dùng thật của Arthur: *"claude code memory — liệu có ai
từng build loại này chưa, và có đáng đọc không?"*

Đó là **hai câu hỏi**, và V2 chỉ phục vụ được một.

- *"Có ai build chưa"* = tồn tại + liên quan. Điểm upvote gần như vô dụng ở
  đây: repo đúng thứ bạn cần có thể chưa ai post lên HN bao giờ.
- *"Có đáng đọc không"* = chất lượng. Đây mới là chỗ điểm cộng đồng làm việc.

## Đã đo trước khi code (2026-08-26)

Nguyên tắc "gọi thật rồi đưa số đo" — và số đo đã **bác bỏ hai đề xuất của
chính tôi**:

**Bỏ: siết bộ lọc precision.** Định yêu cầu khớp ≥2 token thay vì ≥1. Đo trên
9 topic thật:

| Bằng chứng | Số |
|---|---|
| Bài Stuxnet 2012 lọt vào `agent memory` — cái cần chặn | khớp **2/2** token. Không luật đếm token nào chặn được |
| Luật "khớp đủ mọi token" trên `compiler design` | giết *"What can we learn from how compilers are designed?"* — **225đ**, đúng chủ đề |
| Lọc ≥2 token trên GitHub | giết `rohitg00/agentmemory` — **27k sao**, prior art thật |

→ Luật chặt hơn **giết bài đúng nhiều hơn bài sai**. Giữ `matchesAny`. Một bài
lạc đề nhìn thấy ngay bằng mắt (đề năm 2012, domain arstechnica) rẻ hơn nhiều
so với mất bài 225đ.

**Bỏ: lọc Show HN.** Định vứt Show HN vì nó là quảng cáo sản phẩm. Sai — với
câu hỏi "có ai build chưa" thì Show HN **chính là câu trả lời**. Nay chỉ đánh
dấu `[S]` (TUI) / `[Show HN]` (CLI), không lọc. `_tags` của Algolia có
`show_hn` và `ask_hn`, đã verify.

## Ba thứ đã làm

**1. SortRelevance — phím `s` / `-sort relevance`.**

Đo được: query `claude code memory` cho ra gần như toàn bộ kết quả 0-8 điểm.
Khi mọi thứ đều ~0 thì cộng đồng **chưa phán quyết**, nên sắp theo điểm là sắp
theo nhiễu. Bằng chứng cụ thể — sort theo điểm chôn hoàn toàn mấy bài này,
sort theo relevance lôi hết lên top:

- `code.claude.com/docs/en/memory` — **trang docs chính thức**
- *"How Claude Code memory works"*
- *"Claude Code's Local Memory Is a Security Risk"*
- *"File System as Claude Code's Memory"*

Đây **không phải công thức chấm điểm mới**. Nó là chỗ thú nhận rằng lúc này
không có tín hiệu chất lượng nào, nên trả thứ tự về cho nguồn thay vì giả vờ
biết. Đổi sort tính tại chỗ từ `Report.Merged`, không gọi lại API.

**2. GitHub làm nguồn thứ ba — danh sách RIÊNG.**

`api.github.com/search/repositories`, không cần key. **Rate limit thật: 10
request/phút** cho search resource khi không có token (không phải 60/giờ —
đó là core API). Chạm limit thì báo rõ nguyên nhân, và không bao giờ là lỗi
chí mạng.

- **KHÔNG sort theo stars.** Đo: `sort=stars` với `claude code memory` cho top
  3 là repo 243k/80k/69k sao chẳng liên quan gì — đúng kiểu hỏng của
  `order=score` bên lobste.rs. Stars đo độ nổi tiếng của cả repo, không đo mức
  liên quan tới query. Dùng best-match, stars làm ngữ cảnh.
- **KHÔNG trộn vào bảng xếp hạng HN/lobste.rs.** Thang điểm không so được, và
  nó trả lời câu hỏi khác. TUI: phím `tab`. CLI: mục riêng ở cuối.
- Repo `archived` vẫn giữ, chỉ đánh dấu — "có ai build chưa" thì một repo bỏ
  hoang vẫn là câu trả lời có.

**3. Quy tắc dùng (không phải code).**

- *"Có ai build chưa"* → gõ hẹp và cụ thể, xem pane **repo**, chấp nhận điểm thấp.
- *"Đáng đọc không"* → gõ tên thứ **đã lắng** (`CLAUDE.md`, `agent memory`),
  điểm cao mới có nghĩa.
- **Term mới nổi thì bấm `s`.** Điểm chưa hình thành.
- **Search bằng cơ chế, đừng search bằng tên term.** `llm agent loop` ra bài
  447đ; `ai agent harness` ra toàn Show HN dưới 10đ. Term mới chưa kịp tích
  điểm, cơ chế thì đã có người viết hay từ lâu.

## Giới hạn không sửa được bằng code

**HN thưởng cho drama.** Query `claude code` cho ra 2445đ (steganography),
2095đ (source leak), 1364đ (unusable), 1349đ (refuses requests). Upvote đo
**độ đáng bàn**, không đo **độ đáng đọc**. Với topic kỹ thuật thuần hai thứ đó
trùng nhau; với topic có tính thời sự thì tách rất xa. Đây là cái giá của việc
mượn phán quyết cộng đồng — nhận lấy, đừng chữa bằng heuristic.

**Tag map lệch với topic không phải ngôn ngữ.** `claude code` → tag
`editors, vscode` (vì token "code"). Vô hại vì lobste.rs không góp kết quả,
nhưng biết để đừng tin dòng tag đó.

---

# V4 — Hai nhánh: Articles/Research và Repos (kế hoạch — 2026-08-27)

> **Trạng thái: ĐÃ BUILD (2026-08-27).** Mục "Đã verify" là số đo thật trước
> khi code. Mục "Rủi ro đã biết" là chỗ đã đo ra vấn đề nhưng vẫn giữ trong
> thiết kế. Mục "Đã chốt khi build" ở cuối ghi những chỗ **thiết kế này sai**
> và code đi khác — đọc mục đó trước khi tin phần trên.

## Kiến trúc tổng thể

Hai nhánh **hoàn toàn tách biệt**: không dùng chung pipeline, không merge score.

```
albert articles <topic>      # nội dung để đọc: article, paper
albert repos --lang <X>      # project/tool đang trending
```

Lý do tách: article và repo khác bản chất (văn bản để đọc vs project để dùng).
So "upvote bài viết" với "star repo" trên cùng thang là vô nghĩa — đây chính là
kết luận đã rút ở V3 khi tách pane GitHub bằng phím `tab`.

---

## Phần 1 — Articles & research

```
candidate generation → dedup/canonicalize → source-specific scoring → ranking
```

**Nguyên tắc:** rank riêng theo từng `content_type`, KHÔNG gộp 1 điểm chung.
Merge chỉ ở tầng hiển thị (group-by-type / interleave theo topic), không merge ở
tầng điểm số.

### Nguồn

| # | Nguồn | content_type | Cách lấy | Signal rank |
|---|---|---|---|---|
| 1 | Hacker News | `community-voted` | Algolia Search API (`hn.algolia.com/api/v1/search`) | native upvote (`points`) |
| 2 | lobste.rs | `community-voted` | JSON endpoint `/t/{tag}.json` | native upvote (`score`) |
| 3 | dev.to | `community-voted` | REST `dev.to/api/articles?tag=X&top=N` | `positive_reactions_count` |
| 4 | arXiv | `academic` | Atom feed theo category (`cs.SE`, `cs.DC`…) | không có signal nội tại, chỉ candidate |
| 5 | Semantic Scholar | enrich cho #4 | API qua arXiv ID | enrich (`citationCount`, `tldr`), KHÔNG rank |

Host dev.to đúng là `dev.to/api/...` — `api.dev.to` không resolve.

### Common struct (trước khi dedup)

```
{ url, title, source, source_native_score, published_at, content_type }
```

`source_native_score` chỉ có nghĩa **trong phạm vi một content_type** — không so
điểm dev.to với điểm HN (xem magnitude đo được ở mục Đã verify).

### Dedup/canonicalize — làm tăng dần, không build trước khi thấy vấn đề

1. **URL normalize** (bỏ query/UTM, lowercase host, bỏ `www.`, ép https, bỏ
   trailing slash) — mặc định, chi phí ~0 → **bắt đầu ở đây**
2. Follow `<link rel="canonical">` — chỉ thêm khi đo được nhiều case cross-post
   không canonical trong data thật. Tốn 1 HTTP request/bài, đắt hơn (1) hai bậc.
3. Content fingerprint (hash/embedding) — chưa cần ở scope hiện tại

### Academic — xử lý riêng, không rank theo citation

- **arXiv**: free Atom feed theo category — chỉ là candidate generation, KHÔNG
  có quality signal nội tại (preprint, không peer review)
- **Semantic Scholar**: enrich (`citationCount`, `tldr`) qua arXiv ID, KHÔNG
  dùng để rank — paper mới ~0 citation nên rank theo citation sẽ luôn chôn bài
  mới (cold-start)
- **Cách rank thực tế**: cross-reference với HN/lobste.rs — paper nào được
  submit lên đó thì coi là subset `community-voted`, dùng chính upvote làm
  signal. Paper chưa được nhắc tới → feed chronological riêng, gắn nhãn rõ
  *"chưa có tín hiệu chất lượng"*, **không giả vờ ranking**. Cùng tinh thần
  `SortRelevance` ở V3: không có phán quyết thì thú nhận, đừng chế điểm.

### Trạng thái credential

| Nguồn | Trạng thái |
|---|---|
| Semantic Scholar | **Đang xin API key.** Pool ẩn danh 429 ngay request đầu → chưa có key thì coi như không dùng được. |
| Reddit | **Đang xin API key.** Chưa xếp vào bảng nguồn cho tới khi đo được rate limit thật. |
| dev.to | Có key test, **nhưng key không đổi được gì** — xem "Rủi ro đã biết" #1. |

### Nguồn đã xét và loại

| Nguồn | Lý do loại |
|---|---|
| Medium | Chất lượng giảm từ 2023 (đổi paywall + Partner Program), signal/noise thấp |
| Substack | Không có quality signal công khai — RSS không có like/restack count, không có search cross-publication |
| Company engineering blogs (RSS) | RSS chỉ trả ~10–20 item mới nhất, không lịch sử, không param query ⇒ không phục vụ được `articles <topic>`. Muốn dùng phải poll + index, mâu thuẫn với kiến trúc fetch-and-display. |
| Tildes | Invite-only, volume tech quá nhỏ |
| daily.dev | Không public API cho bên thứ 3, nội dung trùng dev.to/HN |
| Hashnode | Có API nhưng overlap content-type với dev.to, chưa có gap cụ thể |

---

## Phần 2 — Repos (độc lập)

```
GET https://github.com/trending/{language}?since={daily|weekly|monthly}
```

- **Scrape trực tiếp trang official** — không dùng Trendshift hay site trung
  gian nào. Thêm một lớp phụ thuộc fragile không giải quyết vấn đề gì.
- Parse `article.Box-row`: repo name, description, primary language, total
  stars, **stars-gained-trong-kỳ** — số này mới là signal "trending", không
  phải tổng star. (Đúng bài học V3: tổng star đo độ nổi tiếng, không đo mức
  liên quan.)
- Không auth, set `User-Agent` đàng hoàng, delay giữa request nếu gọi nhiều
  language
- **Không lưu DB/lịch sử** — fetch-and-display mỗi lần gọi lệnh
- Test tối thiểu: assert parse >0 repo, để phát hiện sớm khi GitHub đổi HTML

**Quan hệ với GitHub search của V3:** hai thứ trả lời hai câu khác nhau —
`search/repositories` (đang chạy) trả lời *"có ai build X chưa"* **theo query**;
trending trả lời *"tuần này nổi gì"* **không theo query**. Không thay thế nhau.
Chưa quyết giữ cả hai hay bỏ một.

---

## Đã verify bằng call thật (2026-08-27)

| Nguồn | Kết quả |
|---|---|
| **dev.to — có API key** | Auth OK (`/api/users/me` → 200). **Nhưng key không mở được search**: `?q=wal internals` vẫn trả feed mới nhất y hệt lúc không auth; `/api/search/feed_content` → **404** (endpoint không tồn tại trong public API). ⇒ dev.to chỉ có `?tag=`. |
| **dev.to — magnitude signal** | `tag=go&top=7` → 10 / 6 / 1 reaction. `top=30` → 38 / 23 / 9. HN cùng ngày: 559đ. **Cửa sổ 7 ngày quá hẹp**, dùng `top=30` trở lên; kể cả vậy vẫn là tín hiệu mỏng hơn HN hai bậc. |
| **dev.to fields** | ✅ `positive_reactions_count`, `public_reactions_count`, `comments_count` có thật. |
| **GitHub trending** | ✅ `github.com/trending/go?since=weekly` → 200 với UA tự đặt. Parse được 20 `Box-row`, star delta đúng dạng `"366 stars this week"`. |
| **arXiv** | ✅ 200 — **bắt buộc set User-Agent định danh**; UA mặc định của curl → **429**. Phải dùng `https://export.arxiv.org` (http → 301). |
| **Semantic Scholar** | ❌ **429 ngay request đầu, 5/5 lần**, có UA lẫn không. Đang xin key. |
| **lobste.rs tag `ai`** | 75 bài gần nhất của tag `ai` **không bài nào** title chứa claude/memory/anthropic. lobste.rs không phủ chủ đề AI tooling. |

## Rủi ro đã biết — chấp nhận khi build

1. **dev.to không có full-text search, và fail *im lặng*.** `?q=...` trả 200
   nhưng **bỏ qua hẳn param**, trả feed mới nhất (`"Welcome Thread - v390"` cho
   query `wal internals`). **Đã thử lại với API key: y hệt.** Không có endpoint
   search nào trong public API (404).
   ⇒ Chỉ `?tag=X` dùng được, tức dev.to mắc **đúng bệnh tag-matching của
   lobste.rs**: topic không map được tag thì nguồn này rớt. Cần logic map
   topic→tag riêng cho dev.to; khi không map được thì **báo rõ như lobste.rs
   đang làm**, tuyệt đối đừng để `?q=` trả feed sai mà tưởng là kết quả tìm.

2. **Semantic Scholar là optional, không phải dependency.** Pipeline phải chạy
   đúng khi S2 fail — enrich thiếu thì để trống field, không làm hỏng cả nhánh
   academic. Kể cả khi có key.

3. **arXiv cross-ref HN gần như là no-op.** Đo tỉ lệ link arxiv trong kết quả
   HN: `transformer attention` 6/37 (16%), `distributed consensus` 2/50 (4%,
   có bài 343đ), `llm inference` 3/50 (6%). Paper nào có tín hiệu cộng đồng thì
   **search HN đã lấy được rồi**. Net-new của arXiv feed chỉ là danh sách
   chronological không ranking. Build sau cùng, đừng kỳ vọng nó nâng top-10.

## Đã chốt khi build (2026-08-27)

Ba việc trước đây để ngỏ, giờ đã chốt và đã code.

### 1. Ngưỡng điểm sàn HN = 10, min-keep = 5 ✅

`internal/search/threshold.go`. Flag `-min-score` để đổi (0 = tắt).

Đo lại rộng hơn — 12 topic, 326 hit: **median điểm chỉ 2–4**. Đuôi rác không
phải thiểu số, nó là đa số.

| Luật | Giữ /326 | Kết luận |
|---|---|---|
| sàn 10 | 162 (50%) | **chọn** |
| sàn 20 | 140 (43%) | ở topic dày cắt cả bài thật (agent harness 22→16) |
| 5% điểm max | 155 | topic hot (max 682/915) đẩy sàn lên 34/45, cắt bài 29đ/23đ thật |
| Show HN ngưỡng riêng 10/25 | 157 | chỉ khác sàn-10 đúng 5 slot/326 — không đáng thêm code |

**Luật tương đối tệ hơn tuyệt đối** vì nó cắt mạnh nhất đúng lúc topic có nhiều
bài hay nhất. Việc "đừng cắt khi topic chưa có điểm" thì min-keep làm rõ ràng
hơn, và sàn **tự tắt ở `-sort relevance`** — mode đó tồn tại đúng để phục vụ
topic cộng đồng chưa phán quyết.

Sàn CHỈ áp cho HN. lobste.rs thang nhỏ hơn cả chục lần, dùng chung sàn là xoá
sổ nguồn đó.

### 2. `tagAliases` — đã thêm claude/anthropic/gpt/openai/copilot → `ai` ✅

`internal/search/lobsters.go`. Nhưng **số đo cũ vẫn đúng và alias này không
sửa được nó**: lobste.rs không phủ AI tooling. Quan sát sau khi thêm, query
`claude memory` lôi về *"June Framework Memory and storage pricing updates"*
(5đ, tag `ai`) — bài về **giá RAM**, lọt vì extra-token chỉ còn `memory` và
`matchesAny` khớp bất kỳ. Alias mở cửa feed `ai` ra, và rác đi kèm qua cửa đó.
Không nghiêm trọng (điểm 5 thì nằm đáy bảng), nhưng đừng tưởng alias đã fix gì.

### 3. GitHub: GIỮ CẢ HAI ✅

Hai pane riêng, không bỏ cái nào — chúng trả lời hai câu khác nhau:
`search/repositories` = *"có ai build X chưa"* (theo query);
`github.com/trending` = *"tuần này nổi gì"* (KHÔNG theo query).
TUI: `tab` vòng qua **bài → repo → trending**.

Trending không nhận query, nên ngôn ngữ đoán từ token của topic
(`go scheduler` → `/trending/go`); không đoán được thì lấy mọi ngôn ngữ.
Flag `-lang` để ép.

---

## Chỗ thiết kế SAI, code đã đi khác

**1. arXiv: dùng `search_query`, KHÔNG dùng Atom feed theo category.**
Thiết kế trên ghi "Atom feed theo category (`cs.SE`, `cs.DC`…)". Sai — arXiv có
full-text search thật: `search_query=all:"<topic>"&sortBy=relevance`. Đã chạy:
`"speculative decoding"` → 682 kết quả. Feed category chỉ trả paper mới nhất
của cả ngành, chẳng liên quan gì tới topic đang gõ.

Kèm theo đó, bộ lọc phải **chặt hơn HN chứ không giống HN**: đòi **mọi** token
của topic có mặt trong **title** (`matchesAll`, không phải `matchesAny`, và
không xét abstract). Lý do đo được: `go scheduler` với bộ lọc lỏng vẫn lôi về
paper task-scheduling cho data center — abstract nào chẳng có chữ "go". Kết quả
sau khi siết: `go scheduler` → 0 paper, `claude memory` → 0, `speculative
decoding` → 5, `raft consensus` → 5. Đúng ý đồ: **arXiv im lặng khi topic không
phải thuật ngữ nghiên cứu.** Nguồn không có tín hiệu chất lượng thì thà 0 kết
quả còn hơn đoán bừa.

**2. dev.to: KHÔNG dùng `short_summary` làm Description để map tag.**
Thiết kế nói "cần logic map topic→tag riêng giống matchTags". Đúng phần map,
sai phần dữ liệu. `matchTags` dùng lại được nguyên xi (chỉ tham số hoá bảng
alias), nhưng phải **bỏ hẳn `short_summary`**: nó là văn quảng cáo có kể tên
ngôn ngữ. Đo được — summary của tag `githubcopilot` chứa câu *"...works
especially well for Python, JavaScript, TypeScript, Ruby, **Go**, C# and C++"*,
nên `go scheduler` khớp luôn tag đó. dev.to chỉ khớp theo **tên tag + alias**.
(lobste.rs thì ngược lại: description của nó là tên gọi khác của tag, dùng tốt.)

**3. `ContentType` phân nhóm theo THANG ĐIỂM, không theo bản chất nội dung.**
Thiết kế xếp dev.to chung `community-voted` với HN. Không dùng được: cùng là
upvote nhưng dev.to thấp hơn hai bậc (38 reaction vs 559 điểm), trộn chung thì
dev.to chìm nghỉm ở đáy — tức thêm nguồn mà không thêm thông tin. Ba nhóm thật
sự là `TypeVoted` (HN+lobste.rs) / `TypeBlog` (dev.to) / `TypeAcademic` (arXiv).

Rank riêng từng nhóm, nối lại **chỉ ở `Report.compose`** — tầng hiển thị.
`topN` chỉ cắt nhóm chính; hai nhóm phụ lục có hạn riêng `appendixMax = 5`,
nếu không thì `-n` nhỏ sẽ xoá sạch nhóm chính rồi để dev.to chiếm màn hình.
arXiv in điểm là **`—` chứ không phải `0`**: số 0 trông như bị chấm 0 điểm,
còn đây là chưa ai chấm cả.

Dedup chéo nhóm (`dropSeen`) chỉ dùng bậc 1 — chuẩn hoá URL. Bản HN thắng, bản
dev.to cross-post biến mất. Chưa đo được case nào cần bậc 2 (`rel=canonical`).

---

## Còn lại

- **Semantic Scholar**: có API key nhưng chưa nối. Khi nối: enrich
  `citationCount`/`tldr` cho nhánh arXiv, **KHÔNG rank** — paper mới ~0
  citation nên rank kiểu đó chôn sống bài mới. Phải optional thật: S2 fail thì
  field để trống, không làm hỏng cả nhánh academic.
- **Reddit**: đang xin key, chưa xếp vào bảng nguồn.
- **Scrape trending là chỗ dễ vỡ nhất** trong cả pipeline. Có test offline với
  fixture HTML thật (`internal/search/testdata/trending-go.html`) để biết ngay
  khi GitHub đổi markup. Parse 0 dòng thì **báo lỗi**, không trả danh sách rỗng
  im lặng — trending thì luôn có repo, 0 dòng gần như chắc chắn là parser hỏng.
