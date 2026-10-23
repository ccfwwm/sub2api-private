# Sub2API v1.3.2

媒体分组新增可用状态。管理员可编辑为“可用”或“不可用”，用户在媒体模型能力页看到对应状态图标；既有分组通过迁移默认设为可用。状态仅控制展示，不改变分组成员、媒体调用、定价或计费。

数据库变更由受审迁移 `251_media_group_availability.sql` 增加 `groups.media_availability`，默认值为 `available`，并通过数据库约束限制为 `available` / `unavailable`。发布资产携带 SQL 校验值，部署管理器只接受此固定加列语句，并先完成备份副本恢复演练。

回归覆盖迁移 SQL 精确审核、checksum、迁移清单与 release assets staging，以及管理员状态编辑和用户状态展示。
