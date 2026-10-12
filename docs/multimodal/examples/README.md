# 调用示例

Python 3.10+（仅标准库）或 Node.js 20+（内置fetch/FormData）。新增路由待开发，真实调用前需能力与价卡发布并完成渠道验证；示例不会自动申请供应商权限。不要将测试模板或示例价格写到生产。

PowerShell 设置 `SUB2API_BASE_URL` 为自己的HTTPS**站点根地址**，不带`/v1`、签名参数或凭证；`SUB2API_API_KEY` 使用自己平台的下游Key。仅localhost允许HTTP用于mock。不要提交环境变量内容。下面使用Python；Node将命令前缀换为`node docs/multimodal/examples/media_client.mjs`，参数一致。

```powershell
$env:SUB2API_BASE_URL = 'https://your-sub2api.example'
# SUB2API_API_KEY 由本地安全环境提供，不在文档填真实值。
python docs/multimodal/examples/media_client.py models
python docs/multimodal/examples/media_client.py estimate --payload estimate.json
python docs/multimodal/examples/media_client.py submit --kind video --payload video.json --state video-task.json
python docs/multimodal/examples/media_client.py resume --state video-task.json --output video.mp4 --max-wait 1200 --poll-interval 20
python docs/multimodal/examples/media_client.py submit --kind audio --payload music.json --state music-task.json
python docs/multimodal/examples/media_client.py resume --state music-task.json --output song.mp3
python docs/multimodal/examples/media_client.py call --endpoint images/generations --payload image.json --output image-result.json
python docs/multimodal/examples/media_client.py call --endpoint chat/completions --payload music-chat.json --output music-result.json
python docs/multimodal/examples/media_client.py call --endpoint gemini --model gemini-3-pro-image-preview --payload gemini.json --output gemini-result.json
python docs/multimodal/examples/media_client.py edit --payload edit.json --image first.png --image second.png --output edit-result.json
```

`video.json`：`{"model":"00sd2.5","prompt":"镜头缓慢推进","seconds":30}`。不传resolution。LL版本应明确720p。

`estimate.json`：`{"operation":"video","request":{"model":"00sd2.5","prompt":"镜头缓慢推进","seconds":30}}`。

`image.json`/`edit.json`：`{"model":"gpt-image-2","prompt":"浅色科研插画","n":1,"response_format":"url"}`。编辑素材按--image顺序重复image[]字段；客户端生成boundary。

`gemini.json`：`{"contents":[{"role":"user","parts":[{"text":"蓝紫色科研插画"}]}],"generationConfig":{"responseModalities":["IMAGE"],"imageConfig":{"imageSize":"1K"}}}`。自动比例省略；TEXT输出URL是渠道扩展。

`music.json`：`{"model":"lyria-3.5","prompt":"钢琴纯音乐，无人声","instrumental":true,"async":true,"audio_format":"mp3"}`。该专用入口来源仍为404，未验证前不能调用。

`music-chat.json`：`{"model":"lyria-3.5","messages":[{"role":"user","content":"温暖的爵士纯音乐，不要人声"}],"stream":false}`。SSE/歌词等扩展不在已验证示例中；call只解析JSON，不解析SSE。

**恢复行为**：submit创建状态文件后只POST一次。state包含version/base/kind/endpoint、request_hash、idempotency_key、task_id、status，不保存Key、完整提示词/素材/下载签名地址。重复submit相同state会在发请求前失败，不覆写。恢复只GET，不发POST。没有ID的unknown/pending_submission需要用hash/幂等键核对平台日志，不能自动再次提交。默认20秒轮询，429/临时错误退避且遵守Retry-After；本地窗口结束保留state。进度不等于剩余时间。

state是客户端恢复辅助，不能代替未来后台DB任务。两个脚本可交叉resume同一个state，但不能同时使用：旁边`.lock`采用排他创建；强杀后可能留下锁，先确认没有进程在操作此state，才手动移除该**确切锁文件**。state的首次写入中断也可能残缺，此时检查文件并人工核对，不自动删除重提。不要删除unknown记录来解决报错。

查询失败不改变生成成功/失败语义，只有明确status=failed才停为失败。下载通过同站点鉴权content，不跟随任何重定向；200/有效媒体MIME及基础魔数校验后以临时文件原子落盘。客户端默认最大256MiB下载、64MiB JSON/编辑上传，实际渠道可能更小。魔数检查不能证明完整视频能播放；真实验收仍需媒体探测。GET失败可再次resume，不会生成新任务。

models/estimate打印用户侧结果；call/edit保存完整响应，可能包含签名链接/Base64，应按私人结果保存，不能提交Git或公开。同步call/edit亦只POST一次，没有可恢复ID时不自动重试，需核对服务器；其结果文件不等同异步state。当前示例不演示批量收费调用、自动重提、Range断点下载、SSE或本地账户/成本覆盖。

验证（仅本地mock，使用假Key）：

```powershell
python docs/multimodal/generate_spec.py
python docs/multimodal/verify_spec.py
node --check docs/multimodal/examples/media_client.mjs
```

验证脚本依赖PyYAML/jsonschema；验证完整OpenAPI需openapi-spec-validator。可在本地独立Python环境中运行`python -m pip install -r docs/multimodal/requirements-spec.txt`安装本次验证版本。依赖不存在会明确报错，不把只解析YAML算完整OpenAPI验收。所有mock只绑定127.0.0.1并使用临时目录，不读取生产凭证。
