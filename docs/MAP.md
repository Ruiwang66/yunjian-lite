# 前台与后台服务器分布地图

前台 `/` 和后台概览 `/admin` 共用地图组件。前台地图默认收起，点击概览标题旁的地图图标展开，再次点击收起；后台概览地图始终显示。`/servers` 专门展示服务器列表、搜索和状态筛选，不再重复概览统计、地图和最近活动。绿色表示该国家或地区至少有一台在线服务器，灰绿色表示全部离线或待连接。支持鼠标、键盘和地区标签选择，展示该地区服务器数量、在线数量与详情链接；小地区以地图标记补充，全部地区也可从下方标签选择。

在「添加服务器」选择国家/地区，或在后台地图「设置地区」选择已有服务器和地区。清空地区后归为未设置。采用数据库增量迁移，旧服务器默认为空，原名称、密钥、指标和状态保留；监控上报和重命名不会改动管理员设置的地区。

地区由后台管理员或运维人员手动设置，不自动查询公网 IP 地理位置；viewer 和未登录访客不能编辑。新接口 `PATCH /api/servers/:id/location` 接受 `country_code`，验证地图代码表、会话、角色和来源，记录审计并通知页面刷新。创建服务器兼容原 `{name}` 参数，也接受可选 `country_code`。

公开接口只新增国家/地区代码，地图显示固定地理形状或地区标记，不公开服务器 IP、城市、机房坐标、密钥、用户或运维信息。地图资源从本站加载，运行时不连接第三方地图、定位服务或瓦片服务器。

## 数据与许可

- 底图为 Natural Earth 1:110m Countries，1:50m Countries 补充小地区的标记位置和中文标签。数据为公共领域：[Natural Earth 使用条款](https://www.naturalearthdata.com/about/terms-of-use/)。
- 原始数据来源：[110m GeoJSON](https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_110m_admin_0_countries.geojson)、[50m GeoJSON](https://github.com/nvkelso/natural-earth-vector/blob/master/geojson/ne_50m_admin_0_countries.geojson)。原始 50m 数据 SHA-256：`3e458fc036ad0a66411f2c1e6cac49c5d7bfb81cb1123bc513b22511a2b7fdeb`。
- 使用 Robinson 投影，在生成阶段计算 SVG 路径。正向算法改编自 d3-geo-projection，ISC 许可保存在 `web/src/components/world-map-license.txt`。运行时不引入 D3 或地图 SDK。
- 前端地图资源 `world-map.json` 约 164 KiB，237 个可选国家/地区；地图组件独立按需加载，压缩传输约 68 KiB。地图只用于服务器分布展示，不用于精确位置、边界或面积测量。

## 本地验证

`tests/map-distribution.mjs` 覆盖分组、在线计数、未知地区、空状态、地图与代码表一致性。Go 测试覆盖地区编辑权限、旧参数兼容、地区代码验证、公开投影隐私、地区保存和重命名保留；旧库迁移测试确认不会虚构位置。

以下为 2026-10-13 的本地验证记录；地图代码已纳入 2026-10-19 的 v0.3 GitHub 源码更新。本次发布不重新部署云端。

2026-10-13 本地验证完成：Go 接口、迁移及 shared 测试和 vet 通过，地图分组及代码表一致性测试通过，Vite 构建通过。浏览器在 `http://127.0.0.1:5053/` 验证前台地图、点击地区查看服务器、后台选择并保存地区、前台同步更新；演示服务器测试从中国改为美国后已恢复中国。旧服务器需自行设置真实地区，演示位置不表示实际地理位置。
