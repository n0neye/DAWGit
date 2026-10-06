# DAWGit

> **本專案已搬到 [R3V](https://github.com/nonlabhq/r3v)。** DAWGit 不再開發，這個 repository 已封存。

[English](README.md) | **繁體中文**

專為音樂人設計的 Ableton Live 版本管理與協作工具。

> **開發中（WIP）。** DAWGit 目前是 Windows 上的早期預覽版，已在 Ableton Live 12 上測試。1.0 之前可能還有粗糙之處與變動，重要的專案請自行另外備份。

![DAWGit 顯示專案中變更的音軌，準備保存版本](docs/images/screenshot.png)

## 為什麼用 DAWGit

**專為音樂人設計。** 照常在 Live 裡按 Ctrl+S，DAWGit 就會逐軌列出改了什麼。用一句話保存版本、隨時回到任何一個版本，也可以開 branch 嘗試新點子。團員一起改同一首歌時，DAWGit 會逐軌合併，只有兩人改到同一條音軌時才會請你選擇。不需要懂 Git。

**Sample 自動跟著走。** 硬碟上任何位置的 sample 都會跟著每個版本保存，並在團員的電腦上自動重新連結。不用再 *Collect All and Save*，也不會再出現「media files missing」。

**開源，儲存空間是你自己的。** DAWGit 免費、以 MIT 授權開源。團隊的歌曲存放在你們自己的 S3 相容 bucket（Cloudflare R2、Amazon S3、MinIO…），不經過我們的伺服器。小團隊通常在 Cloudflare R2 的免費額度內，也不需要任何電腦保持開機。

## 限制

- **只支援 Ableton Live**，目前只有 Windows 版（macOS 規劃中）。
- **Plugin 不會同步。** DAWGit 不會複製 plugin，也無法確認 plugin 的版本；plugin 從專案外載入的 sample（例如 Kontakt、Serum 裡的）也無法自動收集。如果團員沒有相同的 plugin，建議先把那些音軌 Freeze 再分享，或優先使用 Live 內建的 Simpler、Sampler、Drum Rack，這些會完整同步。另外，有些 plugin 即使沒動也會存下變動的狀態，可能讓該音軌顯示為有變更。
- **拿到 Connection code 的人就有完整權限。** Connection code 裡含有儲存空間的金鑰：任何拿到的人都能讀取、修改、刪除團隊所有的歌曲。請私下傳送，只分享給信任的成員。如果外流，請建立新的金鑰並傳送新的 code。
- **早期預覽版。** 舊版本與已刪除的歌曲目前還不會從儲存空間清除，也還沒有自動更新（有新版本時 DAWGit 會通知你）。

## 開始使用

從 [Releases](../../releases) 頁面下載 `DAWGit-<version>-setup.exe` 並執行（需要 Windows 10 21H2 以上或 Windows 11，不需要系統管理員權限）。安裝程式尚未簽章，如果 Windows 顯示「Windows 已保護您的電腦」，請點 **其他資訊 → 仍要執行**。

**自己使用：** 選 **Just keep versions on this computer**，選擇你的 Ableton 專案資料夾，一邊工作一邊保存版本。之後隨時可以分享給團隊。

**建立團隊（由一個人設定）：** 選 **Create a team**，照步驟建立 Cloudflare R2 的 bucket 和金鑰（大約 5 分鐘），或填入其他 S3 相容儲存空間。DAWGit 會檢查設定，並給你一組 **Connection code** 傳給團員。

**加入團隊：** 選 **Join a team**，貼上收到的 Connection code，接著下載團隊的歌曲或加入你自己的專案。

詳細步驟與日常使用請見 [團隊架設指南](docs/team-setup.md)（英文）。

## 更多

- [團隊架設指南](docs/team-setup.md)（英文）
- [命令列工具](docs/cli.md)（英文）
- [給 AI Agent 的使用說明](docs/agents.md)（英文；Claude Code、Codex、Cursor 等）
- [建置與開發](docs/development.md)（英文）

歡迎到 [Issues](../../issues) 回報問題或提供意見。

## 開發方式

DAWGit 是在 AI 程式助理（Claude）協助下開發的，由維護者主導並審核：每一項變更都經過人閱讀、實際試用後才決定採用。因為它保管的是大家的作品，所以靠的是檢查，而不是信任：

- Live set 的合併與差異比對，由 golden 檔案（來自另一個獨立的初版實作）逐位元組把關；儲存格式（內容 hash、分塊邊界、版本紀錄）有 golden 測試，絕不做舊版讀不懂的改變。
- 端對端測試會用真實的雲端儲存跑真實的團隊流程（存檔、更新、衝突、中斷的上傳、還原）。
- Live 開著專案時絕不改寫專案檔，也絕不刪除隊友的作品：更新會保留你尚未提交的變更，儲存空間清理和還原只動沒有任何版本使用的、或缺少的東西。

詳見 [建置與開發](docs/development.md) 和 [docs/design](docs/design) 裡的設計筆記（英文）。

## 授權

[MIT](LICENSE)
