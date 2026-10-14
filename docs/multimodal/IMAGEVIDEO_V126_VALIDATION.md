# v1.2.6 文档及 GPT 编辑验证

- 默认浅色，深色开关独立保存；8 组目录、23 个章节；参数、地址、请求及响应按场景排列。
- GPT `/v1/images/edits` 提供单图、多图和遮罩示例；公开内容与复制 Markdown 共源，无参考站品牌和内部核验措辞。
- 修复从模型映射创建的 GPT 编辑配置缺少上传边界：缺省允许 2 张 PNG/JPEG/WebP，32 MiB；已有显式数量（含 0）、格式和大小保持原配置。
- 生产账号 2114 的六个 GPT 别名通过管理 API 补齐同样上传边界；修改前 extra 备份保留在服务器 root-only 目录。服务端正常规范化其他默认字段。
- 授权用户 Key 真实单图任务 `mt_9ec4dc92-e7b9-4da5-a894-929e3dd2e4a9` 和双图任务 `mt_fa423e6f-0b85-433e-8fb7-101124e30db4` 均 completed / settled；PNG 鉴权下载 200。各 0.04 USD，共 0.08 USD。未测试其他模型的收费生成；来源示例仅转换为本站当前支持场景。
- 前端相关 10 项用例、国际化 3 项用例、类型检查、lint 与生产构建通过。
- Go `internal/media` 通过，新增上传单图/双图、数量超限和显式禁用保留用例。首次已有适配器 server_error 用例瞬时失败，未修改代码重复后通过。
- Python / JavaScript 客户端合同 4 项测试通过：同步/异步/202 回退、双图真实 multipart 编码、unknown 停止、外域拒绝与断点下载。
- 独立浏览器对构建产物检查：23 章节、默认浅色、深色切换、复制全文 53716 字符、390px 手机音乐锚点与目录关闭、无横向溢出、无 JS 错误。
- 截图：`output/imagevideo/screenshots/v126-docs-light.png`、`v126-gpt-edit.png`、`v126-docs-dark.png`、`v126-docs-mobile.png`。

生产发布结果在上线后单独补充，不将构建产物验证称为已上线。
