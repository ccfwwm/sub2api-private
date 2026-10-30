# v1.3.19 验证记录

## 发布前

- 代码基于 `42a077466`，保留 `.playwright-mcp/` 本地目录，不纳入发布。
- Foxrouter `gemini-omni-flash` 真实契约已验证：`Alias.images` 接受一个或两个 Base64 Data URL 字符串数组；裸字符串会被 HTTP 400 拒绝。凭证未写入仓库。
- 本地媒体模板、原生 HTTP 合同、媒体诊断界面和文档脚本回归已通过，详见 `VALIDATION-v1.3.18.md` 的测试清单。

## 生产

- 正式 Release 与生产部署：待记录。
- 生产账号 2150 的 `gemini-omni-flash` 绑定：待记录。
- 三个生产入口版本、单图/双图标准任务提交、状态查询与成品下载：待记录。
- 数据库和 Redis 容器保护状态：待记录。
