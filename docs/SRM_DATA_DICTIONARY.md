# CRM 库 Srm 前缀数据字典

阅读版，内容与 [`srm_dictionary.json`](srm_dictionary.json) 相同。`dbconn` 的 schema 读 JSON，不读本文件。

- 规模：33 张表，512 个字段
- 物理外键：无。表间关联以字段名和 `relations` 为准
- 行数：采集时 `information_schema.TABLES.TABLE_ROWS` 的 InnoDB 估算值，不是精确 COUNT
- 本文件只含结构，不含业务数据

## 给 AI 的查询约定

- 只读查询，表名必须原样使用（`Srm_` 开头，大小写敏感）
- 多数表用 `IsDelete`（`bit(1)`，`0` 正常、`1` 删除）做软删除。查当前有效数据时加 `IsDelete = 0`
- `IsDelete_Mark` 常与 `IsDelete` 一起进入主键或唯一键：未删除时为 `0001-01-01`，删除后写入删除时间，用来允许同一业务键多次软删
- `SysNo` 是各表自己的自增号，不能用来跨表关联
- 人员字段（`OwnerId`、`CreateById`、`ModifyById` 等）是 ERP 人员 Id，不在 `Srm` 表内
- `VendorLabel` 在多张表里既是业务标签，也是联合主键的一部分。过滤供应商时要同时带上标签，避免串数据
- 注释里的枚举（如 `0/1/2/4`）以字段 `COLUMN_COMMENT` 为准，不要自行发明取值
- 逗号包裹的列表字段（如 `,1,2,`）按字符串存储，匹配时用 `LIKE '%,值,%'`

## 表目录

| 表名 | 说明 | 约行数 | 主键 |
| --- | --- | ---: | --- |
| `Srm_BusinessPaymentInfo` | 业务付款流程v2.0,v3.0 | 2390 | SysNo |
| `Srm_ContactInfo` | 供应商联系人信息表 | 2006 | SysNo |
| `Srm_IndustryServiceType` | 供应商行业及服务类型 | 72 | SysNo |
| `Srm_IndustryServiceTypeConfig` | 供应商行业及服务类型配置表 | 113 | SysNo |
| `Srm_IndustryServiceTypeEvaluateDimensions` | 评价维度 | 436 | SysNo |
| `Srm_IndustryServiceTypeEvaluateRules` | 评价规则 | 113 | SysNo |
| `Srm_PayInfo` | 供应商收款信息表 | 1989 | SysNo |
| `Srm_RatingQuality` | 供应商质量评级表 | 1375 | SysNo |
| `Srm_RatingQualityAdjust` | 供应商质量评级调整表 | 1788 | SysNo |
| `Srm_RatingQualityDetail` | 供应商质量评级明细表 | 10591 | SysNo |
| `Srm_RatingQualityDetailAdjust` | 供应商质量评级明细调整表 | 11880 | SysNo |
| `Srm_RatingTarget` | 供应商评级模版指标表 | 103 | SysNo |
| `Srm_RatingTargetConfig` | 供应商评级模版指标配置表 | 466 | SysNo |
| `Srm_RatingTargetConfigHistory` | 供应商评级模版指标配置历史表 | 1760 | SysNo |
| `Srm_RatingTargetHistory` | 供应商评级模版指标历史表 | 397 | SysNo |
| `Srm_RatingTemplate` | 供应商评级模版表 | 11 | SysNo |
| `Srm_RatingTemplateHistory` | 供应商评级模版历史表 | 43 | SysNo |
| `Srm_SpecialApprovalReport` | 特批报备 | 102 | SysNo |
| `Srm_SpecialApprovalReport_Comment` | 特批评论表 | 19 | SysNo |
| `Srm_UserLabelAuth` | 用户标签权限表 | 45 | SysNo |
| `Srm_UserTreeNode` | 用户组织树节点(按NodeId UUID做数据可见范围) | 126 | SysNo |
| `Srm_VendorContract` | 供应商合同表 | 804 | VendorContractId |
| `Srm_VendorEvaluateInvite` | 评价邀请记录表 | 37 | SysNo |
| `Srm_VendorEvaluateInviteItem` | 评价邀请记录表明细 | 551 | SysNo |
| `Srm_VendorEvaluateRecord` | 供应商评价记录 | 590 | SysNo |
| `Srm_VendorEvaluateRecordData` | 供应商评价记录明细 | 2151 | SysNo |
| `Srm_VendorInfo` | 供应商基本信息表 | 2183 | VendorId, VendorLabel, IsDelete, IsDelete_Mark |
| `Srm_VendorOperationRecord` | 供应商操作记录表 | 1865 | SysNo |
| `Srm_VendorPaymentInfo` | 供应商付款 | 256 | SysNo |
| `Srm_VendorPolicy` | 供应商政策表 | 1815 | VendorPolicyId |
| `Srm_VendorWhite` | 供应商白名单表 | 65 | SysNo |
| `Srm_Vendor_Draft` | 供应商草稿表 | 167 | SysNo |
| `Srm_Vendor_Main_Info` | 供应商基本信息主体表 | 2198 | SysNo |

## 跨表关联键

没有外键。下面列出在多张表中出现、且至少在一张表上是主键或唯一键的字段，写 SQL 时优先用它们关联。

| 字段 | 出现的表（键类型） |
| --- | --- |
| `VendorLabel` | `Srm_BusinessPaymentInfo`, `Srm_ContactInfo`, `Srm_IndustryServiceTypeConfig`, `Srm_IndustryServiceTypeEvaluateRules`, `Srm_PayInfo`, `Srm_RatingQuality`, `Srm_SpecialApprovalReport`, `Srm_UserTreeNode`, `Srm_VendorContract`, `Srm_VendorEvaluateInviteItem`, `Srm_VendorEvaluateRecord`, `Srm_VendorInfo`(PRI), `Srm_VendorOperationRecord`, `Srm_VendorPaymentInfo`, `Srm_VendorPolicy`, `Srm_VendorWhite`, `Srm_Vendor_Draft`, `Srm_Vendor_Main_Info` |
| `VendorId` | `Srm_BusinessPaymentInfo`(MUL), `Srm_ContactInfo`(MUL), `Srm_PayInfo`(MUL), `Srm_RatingQuality`(MUL), `Srm_SpecialApprovalReport`(MUL), `Srm_VendorContract`(MUL), `Srm_VendorEvaluateInviteItem`(MUL), `Srm_VendorEvaluateRecord`(MUL), `Srm_VendorInfo`(PRI), `Srm_VendorOperationRecord`(MUL), `Srm_VendorPaymentInfo`(MUL), `Srm_VendorPolicy`(MUL), `Srm_VendorWhite`(MUL) |
| `VersionNumber` | `Srm_RatingQuality`, `Srm_RatingTarget`, `Srm_RatingTargetConfig`, `Srm_RatingTargetConfigHistory`, `Srm_RatingTargetHistory`, `Srm_RatingTemplate`, `Srm_RatingTemplateHistory`, `Srm_SpecialApprovalReport`(MUL) |
| `WorkflowId` | `Srm_BusinessPaymentInfo`(MUL), `Srm_RatingQuality`(MUL), `Srm_RatingQualityDetail`(MUL), `Srm_SpecialApprovalReport`(MUL), `Srm_SpecialApprovalReport_Comment`(MUL), `Srm_VendorContract`(MUL), `Srm_VendorInfo`, `Srm_VendorPaymentInfo`(MUL) |
| `BusinessId` | `Srm_BusinessPaymentInfo`, `Srm_VendorContract`(MUL), `Srm_VendorOperationRecord`, `Srm_VendorPaymentInfo` |
| `BelongingCostLine` | `Srm_BusinessPaymentInfo`(MUL), `Srm_VendorContract`, `Srm_VendorPaymentInfo`(MUL) |
| `IndustryServiceTypeId` | `Srm_IndustryServiceTypeConfig`, `Srm_IndustryServiceTypeEvaluateRules`(MUL), `Srm_VendorEvaluateRecord` |
| `RuleId` | `Srm_IndustryServiceTypeEvaluateDimensions`(MUL), `Srm_IndustryServiceTypeEvaluateRules`(UNI), `Srm_VendorEvaluateRecord` |
| `VendorContractId` | `Srm_BusinessPaymentInfo`(MUL), `Srm_VendorContract`(PRI), `Srm_VendorPaymentInfo`(MUL) |
| `VendorPolicyId` | `Srm_BusinessPaymentInfo`, `Srm_VendorPaymentInfo`, `Srm_VendorPolicy`(PRI) |
| `BankAccountId` | `Srm_BusinessPaymentInfo`(MUL), `Srm_VendorPaymentInfo`(MUL) |
| `InstanceId` | `Srm_BusinessPaymentInfo`(MUL), `Srm_VendorContract`(MUL) |
| `InviteId` | `Srm_VendorEvaluateInvite`(UNI), `Srm_VendorEvaluateInviteItem`(MUL) |
| `RecordId` | `Srm_VendorEvaluateRecord`(UNI), `Srm_VendorEvaluateRecordData`(MUL) |
| `UserId` | `Srm_UserLabelAuth`, `Srm_UserTreeNode`(MUL) |
| `WorkflowFatherId` | `Srm_RatingQualityAdjust`(MUL), `Srm_RatingQualityDetailAdjust`(MUL) |
| `WorkflowSonId` | `Srm_RatingQualityAdjust`(MUL), `Srm_RatingQualityDetailAdjust`(MUL) |

## 各表字段

### `Srm_BusinessPaymentInfo`

- 说明：业务付款流程v2.0,v3.0
- 约 2390 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `fk_BankAccountId` | 普通 | `BankAccountId` |
| `fk_BelongingCostLine` | 普通 | `BelongingCostLine`, `CostBelongingDepartment`, `Department` |
| `fk_CreateById` | 普通 | `CreateById` |
| `fk_CreateTime` | 普通 | `CreateTime` |
| `fk_InstanceId` | 普通 | `InstanceId` |
| `fk_ProcessCode` | 普通 | `ProcessCode` |
| `fk_VendorContractId` | 普通 | `VendorContractId` |
| `fk_VendorId` | 普通 | `VendorId` |
| `fk_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id |
| 3 | `ApplyType` | int(11) | NO |  | 0 |  | 审批类型:300 业务付款流程v2.0;310 业务付款流程v3.0 |
| 4 | `ProcessCode` | varchar(64) | NO | MUL |  |  | 审批模板的唯一码 |
| 5 | `InstanceId` | varchar(64) | NO | MUL |  |  | 实例Id |
| 6 | `BusinessId` | varchar(128) | NO |  |  |  | 钉钉审批编号 |
| 7 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 8 | `VendorName` | varchar(128) | NO |  |  |  | 供应商名称 |
| 9 | `VendorLabel` | int(11) | NO |  | 0 |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 10 | `VendorContractId` | char(36) | NO | MUL | 00000000-0000-0000-0000-000000000000 |  | 合同Id |
| 11 | `VendorPolicyId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 政策ID |
| 12 | `BelongingCostLine` | int(10) | NO | MUL | 0 |  | 所属成本线 T_CascadeData.SysNo，这里直接写 0 |
| 13 | `BelongingCostLineName_v` | varchar(256) | NO |  |  |  | 所属成本线名称 |
| 14 | `PaymentNature` | int(10) | NO |  | 0 |  | 付款性质Id 1境内付款 2境外付款，这里直接写 0 |
| 15 | `PaymentNatureName_v` | varchar(256) | NO |  |  |  | 付款性质名称 |
| 16 | `CostBelongingDepartment` | int(10) | NO |  | 0 |  | 费用所属部门Id T_CascadeData.SysNo，这里直接写 0 |
| 17 | `CostBelongingDepartmentName_v` | varchar(256) | NO |  |  |  | 费用所属部门名称 |
| 18 | `Department` | int(10) | NO |  | 0 |  | 部门Id T_CascadeData.SysNo，这里直接写 0 |
| 19 | `DepartmentName_v` | varchar(256) | NO |  |  |  | 部门名称 |
| 20 | `CostBelongingCompany` | varchar(128) | NO |  |  |  | 费用所属公司 |
| 21 | `AmountCategory` | varchar(32) | NO |  |  |  | 费用类别 |
| 22 | `AssetNumber` | varchar(128) | NO |  |  |  | 固定资产编号 |
| 23 | `AmountReason` | varchar(256) | NO |  |  |  | 付款事由 |
| 24 | `AmountTotal` | decimal(18,4) | NO |  | 0.0000 |  | 付款总额 |
| 25 | `Currency` | int(10) | NO |  | 0 |  | 币种 1人民币 2美金 3港币 4新加坡币 99 其他 |
| 26 | `CurrencyName_v` | varchar(32) | NO |  |  |  | 币种名称 |
| 27 | `BankAccount` | varchar(128) | NO |  |  |  | 开户行 |
| 28 | `BankAccountId` | varchar(128) | NO | MUL |  |  | 银行账号 |
| 29 | `PaymentType` | int(10) | NO |  | 0 |  | 付款方式Id 1转账 2其他，这里直接写 0 |
| 30 | `PaymentTypeName_v` | varchar(64) | NO |  |  |  | 付款方式名称 |
| 31 | `HasInvoice` | bit(1) | NO |  | b'0' |  | 现是否有发票 |
| 32 | `InvoiceFile` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 发票扫描件 |
| 33 | `IsNeedCallBack` | bit(1) | YES |  | b'0' |  | 是否需要回单 |
| 34 | `Remarks` | varchar(512) | YES |  |  |  | 备注 |
| 35 | `Attachment` | char(36) | YES |  | 00000000-0000-0000-0000-000000000000 |  | 附件 |
| 36 | `AuditStatus` | int(11) | NO |  | 0 |  | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回 |
| 37 | `AuditTime` | datetime(3) | YES |  |  |  | 审核时间 |
| 38 | `RemarksIn` | varchar(512) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 39 | `DingDingApproveUrl_v` | varchar(1024) | NO |  |  |  | 钉钉审批的url |
| 40 | `RowCreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 数据行创建时间 |
| 41 | `CreateTime` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 42 | `CreateById` | int(11) | NO | MUL | 0 |  | 创建人Id |
| 43 | `CreateByName_v` | varchar(128) | NO |  |  |  | 创建人姓名 |
| 44 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 45 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 46 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_ContactInfo`

- 说明：供应商联系人信息表
- 约 2006 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_ContactInfo_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_ContactInfo_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 4 | `ContactName` | varchar(32) | NO |  |  |  | 联系人姓名 |
| 5 | `Position` | varchar(32) | NO |  |  |  | 职位 |
| 6 | `Email` | varchar(64) | NO |  |  |  | 邮箱 |
| 7 | `Mobile` | varchar(16) | NO |  |  |  | 手机号 |
| 8 | `CreateTime` | datetime(3) | YES | MUL | CURRENT_TIMESTAMP(3) |  |  |
| 9 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 10 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 11 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 12 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_IndustryServiceType`

- 说明：供应商行业及服务类型
- 约 72 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 行业及服务类型Id，自增，主键 |
| 2 | `IndustryServiceTypeName` | varchar(64) | NO |  |  |  | 行业及服务类型名称 |
| 3 | `Remarks` | varchar(256) | YES |  |  |  | 用户备注信息 |
| 4 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 5 | `CreateById` | int(11) | NO |  |  |  | 创建人Id，ERP用户Id |
| 6 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 7 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 8 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_IndustryServiceTypeConfig`

- 说明：供应商行业及服务类型配置表
- 约 113 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `IndustryServiceTypeId` | int(11) | NO |  |  |  | 行业及服务类型Id，关联 Srm_VendorIndustryServiceType 表 SysNo 字段 |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 4 | `Remarks` | varchar(256) | YES |  |  |  | 用户备注信息 |
| 5 | `RemarksIn` | varchar(256) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 6 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 7 | `CreateById` | int(11) | NO |  |  |  | 创建人Id，ERP用户Id |
| 8 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 9 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 10 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_IndustryServiceTypeEvaluateDimensions`

- 说明：评价维度
- 约 436 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |
| `u_RuleId` | 普通 | `RuleId` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `RuleId` | char(36) | NO | MUL |  |  | 规则ID |
| 3 | `Dimension` | varchar(128) | NO |  |  |  | 维度 |
| 4 | `LevelA` | varchar(256) | NO |  |  |  | 等级1（极差） |
| 5 | `LevelB` | varchar(256) | NO |  |  |  | 等级2（较差） |
| 6 | `LevelC` | varchar(256) | NO |  |  |  | 等级3（合格） |
| 7 | `LevelD` | varchar(256) | NO |  |  |  | 等级4（优秀） |
| 8 | `LevelE` | varchar(256) | NO |  |  |  | 等级5（卓越） |
| 9 | `Weight` | decimal(10,2) | NO |  |  |  | 权重（%） |

### `Srm_IndustryServiceTypeEvaluateRules`

- 说明：评价规则
- 约 113 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IndustryServiceTypeId_VendorLabel` | 普通 | `IndustryServiceTypeId`, `VendorLabel` |
| `PRIMARY` | 主键 | `SysNo` |
| `u_RuleId` | 唯一 | `RuleId` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `EvaluateType` | int(10) | YES |  | 0 |  | 评价类型 1 服务评价  2 财务评价 |
| 3 | `VendorLabel` | int(10) | NO |  |  |  | 标签 |
| 4 | `IndustryServiceTypeId` | int(10) | NO | MUL |  |  | 行业ID |
| 5 | `RuleId` | char(36) | NO | UNI |  |  | 规则ID |
| 6 | `Description` | varchar(256) | YES |  |  |  | 描述 |
| 7 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 8 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |

### `Srm_PayInfo`

- 说明：供应商收款信息表
- 约 1989 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_PayInfo_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_PayInfo_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 4 | `BankName` | varchar(256) | NO |  |  |  | 银行名称 |
| 5 | `BankAccount` | varchar(64) | YES |  |  |  | 银行账户 |
| 6 | `CreateTime` | datetime(3) | YES | MUL | CURRENT_TIMESTAMP(3) |  |  |
| 7 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 8 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 9 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 10 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingQuality`

- 说明：供应商质量评级表
- 约 1375 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_RatingQuality_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_RatingQuality_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `IX_Srm_RatingQuality_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id |
| 3 | `VersionNumber` | char(64) | NO |  |  |  | 版本号，格式：V2022-06-16 10:37:51 |
| 4 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 5 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 6 | `AdjustContent` | varchar(512) | YES |  |  |  | 调整事项 |
| 7 | `AdjustContentScore` | decimal(18,2) | NO |  | 0.00 |  | 调整事项，评分 |
| 8 | `Legal_Rating` | varchar(2) | YES |  |  |  | 质量评级 A B C |
| 9 | `Legal_ComScore` | decimal(18,2) | NO |  | 0.00 |  | 综合评分 |
| 10 | `Legal_Remarks` | varchar(512) | NO |  |  |  | 法务备注 |
| 11 | `RatingStatus` | int(11) | NO |  | 0 |  | 评级状态 0 未准入 1 新建 2 准入审批中 4 准入通过，等待评级 6 已准入，已评级  -1 驳回 -2 撤回 |
| 12 | `RemarksIn` | varchar(256) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 13 | `CreateTime` | datetime(3) | YES | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 14 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 15 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 16 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 17 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingQualityAdjust`

- 说明：供应商质量评级调整表
- 约 1788 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_RatingQualityAdjust_WorkflowFatherId` | 普通 | `WorkflowFatherId` |
| `IX_Srm_RatingQualityAdjust_WorkflowSonId` | 普通 | `WorkflowSonId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `WorkflowFatherId` | char(36) | NO | MUL |  |  | 父工作流Id |
| 3 | `WorkflowSonId` | char(36) | NO | MUL |  |  | 子工作流Id |
| 4 | `AdjustContent` | varchar(512) | YES |  |  |  | 调整事项 |
| 5 | `AdjustContentScore` | decimal(18,2) | NO |  | 0.00 |  | 调整事项，评分 |
| 6 | `Legal_Rating` | varchar(2) | YES |  |  |  | 质量评级 A B C |
| 7 | `Legal_ComScore` | decimal(18,2) | NO |  | 0.00 |  | 综合评分 |
| 8 | `Legal_Remarks` | varchar(512) | NO |  |  |  | 法务备注 |
| 9 | `RemarksIn` | varchar(256) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 10 | `CreateTime` | datetime(3) | YES |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 11 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 12 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 13 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 14 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingQualityDetail`

- 说明：供应商质量评级明细表
- 约 10591 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_RatingQualityDetail_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id |
| 3 | `TemplateVersionNumber` | char(64) | YES |  |  |  | 模版版本号，格式：V2022-06-16 10:37:51 |
| 4 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 5 | `TemplateName` | varchar(256) | NO |  |  |  | 模版名称 |
| 6 | `TargetId` | char(36) | NO |  |  |  | 指标Id |
| 7 | `TargetName` | varchar(256) | NO |  |  |  | 指标名称 |
| 8 | `TargetWeightRatio` | decimal(10,2) | NO |  | 0.00 |  | 指标权重，百分比 |
| 9 | `TargetIsQccTarget` | int(11) | NO |  | 0 |  | 指标是否是企查查指标 0 不是 1 是 |
| 10 | `TargetSort` | int(11) | NO |  | 0 |  | 指标排序，数值越小越靠前 |
| 11 | `TargetItemId` | char(36) | NO |  |  |  | 指标项Id |
| 12 | `TargetItemName` | varchar(128) | NO |  |  |  | 指标项 |
| 13 | `TargetItemScore` | int(11) | NO |  |  |  | 指标项分数 |
| 14 | `CreateTime` | datetime(3) | YES |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 15 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 16 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 17 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 18 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingQualityDetailAdjust`

- 说明：供应商质量评级明细调整表
- 约 11880 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_RatingQualityDetailAdjust_WorkflowFatherId` | 普通 | `WorkflowFatherId` |
| `IX_Srm_RatingQualityDetailAdjust_WorkflowSonId` | 普通 | `WorkflowSonId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `WorkflowFatherId` | char(36) | NO | MUL |  |  | 父工作流Id |
| 3 | `WorkflowSonId` | char(36) | NO | MUL |  |  | 子工作流Id |
| 4 | `TemplateVersionNumber` | char(64) | YES |  |  |  | 模版版本号，格式：V2022-06-16 10:37:51 |
| 5 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 6 | `TemplateName` | varchar(256) | NO |  |  |  | 模版名称 |
| 7 | `TargetId` | char(36) | NO |  |  |  | 指标Id |
| 8 | `TargetName` | varchar(256) | NO |  |  |  | 指标名称 |
| 9 | `TargetWeightRatio` | decimal(10,2) | NO |  | 0.00 |  | 指标权重，百分比 |
| 10 | `TargetIsQccTarget` | int(11) | NO |  | 0 |  | 指标是否是企查查指标 0 不是 1 是 |
| 11 | `TargetSort` | int(11) | NO |  | 0 |  | 指标排序，数值越小越靠前 |
| 12 | `TargetItemId` | char(36) | NO |  |  |  | 指标项Id |
| 13 | `TargetItemName` | varchar(128) | NO |  |  |  | 指标项 |
| 14 | `TargetItemScore` | int(11) | NO |  |  |  | 指标项分数 |
| 15 | `CreateTime` | datetime(3) | YES |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 16 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 17 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingTarget`

- 说明：供应商评级模版指标表
- 约 103 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VersionNumber` | char(64) | YES |  |  |  | 版本号，格式：V2022-06-16 10:37:51 |
| 3 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 4 | `TargetId` | char(36) | NO |  |  |  | 指标Id |
| 5 | `TargetName` | varchar(256) | NO |  |  |  | 指标名称 |
| 6 | `WeightRatio` | decimal(10,2) | NO |  | 0.00 |  | 权重，百分比 |
| 7 | `IsQccTarget` | int(11) | NO |  | 0 |  | 是否是企查查指标 0 不是 1 是 |
| 8 | `Sort` | int(11) | NO |  | 0 |  | 排序，数值越小越靠前 |
| 9 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingTargetConfig`

- 说明：供应商评级模版指标配置表
- 约 466 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VersionNumber` | char(64) | YES |  |  |  | 版本号，格式：V2022-06-16 10:37:51 |
| 3 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 4 | `TargetId` | char(36) | NO |  |  |  | 指标Id |
| 5 | `TargetItemId` | char(36) | NO |  |  |  | 指标项Id |
| 6 | `TargetItemName` | varchar(128) | NO |  |  |  | 指标项 |
| 7 | `TargetItemScore` | int(11) | NO |  |  |  | 指标项分数 |
| 8 | `Sort` | int(11) | NO |  | 0 |  | 排序，数值越小越靠前 |
| 9 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingTargetConfigHistory`

- 说明：供应商评级模版指标配置历史表
- 约 1760 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VersionNumber` | char(64) | YES |  |  |  | 版本号，格式：V2022-06-16 10:37:51 |
| 3 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 4 | `TargetId` | char(36) | NO |  |  |  | 指标Id |
| 5 | `TargetItemId` | char(36) | NO |  |  |  | 指标项Id |
| 6 | `TargetItemName` | varchar(128) | NO |  |  |  | 指标项 |
| 7 | `TargetItemScore` | int(11) | NO |  |  |  | 指标项分数 |
| 8 | `Sort` | int(11) | NO |  |  |  | 排序，数值越小越靠前 |
| 9 | `IsDelete` | bit(1) | NO |  |  |  | 是否删除 0 正常  1 删除 |
| 10 | `RowCreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 数据行创建时间 |
| 11 | `RowCreateById` | int(11) | NO |  |  |  | 数据行创建人Id，ERP用户Id |

### `Srm_RatingTargetHistory`

- 说明：供应商评级模版指标历史表
- 约 397 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VersionNumber` | char(64) | YES |  |  |  | 版本号，格式：V2022-06-16 10:37:51 |
| 3 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 4 | `TargetId` | char(36) | NO |  |  |  | 指标Id |
| 5 | `TargetName` | varchar(256) | NO |  |  |  | 指标名称 |
| 6 | `WeightRatio` | decimal(10,2) | NO |  |  |  | 权重，百分比 |
| 7 | `IsQccTarget` | int(11) | NO |  |  |  | 是否是企查查指标 0 不是 1 是 |
| 8 | `Sort` | int(11) | NO |  |  |  | 排序，数值越小越靠前 |
| 9 | `IsDelete` | bit(1) | NO |  |  |  | 是否删除 0 正常  1 删除 |
| 10 | `RowCreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 数据行创建时间 |
| 11 | `RowCreateById` | int(11) | NO |  |  |  | 数据行创建人Id，ERP用户Id |

### `Srm_RatingTemplate`

- 说明：供应商评级模版表
- 约 11 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键、模版编号 |
| 2 | `VersionNumber` | char(64) | YES |  |  |  | 模版版本号，格式：V2022-06-16 10:37:51 |
| 3 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 4 | `TemplateName` | varchar(256) | NO |  |  |  | 模版名称 |
| 5 | `VendorLabels` | varchar(32) | NO |  |  |  | 模版适用标签，存放格式：,1,2, |
| 6 | `Remarks` | varchar(256) | YES |  |  |  | 用户备注信息 |
| 7 | `Status` | int(11) | NO |  | 0 |  | 是否启用 0 关闭 1 启用 |
| 8 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 9 | `CreateById` | int(11) | NO |  |  |  | 创建人Id，ERP用户Id |
| 10 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 11 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 12 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_RatingTemplateHistory`

- 说明：供应商评级模版历史表
- 约 43 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VersionNumber` | char(64) | YES |  |  |  | 版本号，格式：V2022-06-16 10:37:51 |
| 3 | `TemplateNumber` | int(11) | NO |  |  |  | 模版编号，关联 Srm_RatingTemplate 表 SysNo 字段 |
| 4 | `TemplateId` | char(36) | NO |  |  |  | 模版Id |
| 5 | `TemplateName` | varchar(256) | NO |  |  |  | 模版名称 |
| 6 | `VendorLabels` | varchar(32) | NO |  |  |  | 模版适用标签，存放格式：,1,2, |
| 7 | `Remarks` | varchar(256) | YES |  |  |  | 用户备注信息 |
| 8 | `Status` | int(11) | NO |  |  |  | 1 启用 -1 停用 |
| 9 | `CreateTime` | datetime(3) | NO |  |  |  | 创建时间 |
| 10 | `CreateById` | int(11) | NO |  |  |  | 创建人Id，ERP用户Id |
| 11 | `ModifyTime` | datetime(3) | YES |  |  |  | 修改时间 |
| 12 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 13 | `IsDelete` | bit(1) | NO |  |  |  | 是否删除 0 正常  1 删除 |
| 14 | `RowCreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 数据行创建时间 |
| 15 | `RowCreateById` | int(11) | NO |  |  |  | 数据行创建人Id，ERP用户Id |

### `Srm_SpecialApprovalReport`

- 说明：特批报备
- 约 102 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_T_SrmSpecialApprove_AccountId_VendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `IX_T_SrmSpecialApprove_CreateTime` | 普通 | `CreateTime` |
| `IX_T_SrmSpecialApprove_VersionNumber` | 普通 | `VersionNumber` |
| `IX_T_SrmSpecialApprove_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 主键，自增 |
| 2 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id |
| 3 | `VersionNumber` | varchar(64) | NO | MUL |  |  | 版本号，格式：Vyyyy-MM-dd HH:mm:ss |
| 4 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id，唯一 |
| 5 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 6 | `AuditStatus` | int(11) | NO |  |  |  | 授信评审委员会审批结果 4 通过 -1 评审不通过 |
| 7 | `AuditEnclosureId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 审批附件Id |
| 8 | `OtherEnclosureId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 相关增信文件或实地拜访报告Id |
| 9 | `Remarks` | text | YES |  |  |  | 备注 |
| 10 | `DingTalks` | varchar(1024) | YES |  |  |  | 发送钉钉json数据 |
| 11 | `CreateTime` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 12 | `CreateById` | int(11) | NO |  |  |  | 创建人 |
| 13 | `ModifyTime` | datetime(3) | YES |  |  |  | 修改时间 |
| 14 | `ModifyById` | int(11) | YES |  |  |  | 修改人 |
| 15 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_SpecialApprovalReport_Comment`

- 说明：特批评论表
- 约 19 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_T_SrmSpecialComment_CommentId` | 普通 | `CommentId` |
| `IX_T_SrmSpecialComment_CreateTime` | 普通 | `CreateTime` |
| `IX_T_SrmSpecialComment_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 主键，自增 |
| 2 | `CommentId` | char(36) | NO | MUL |  |  | 评论Id |
| 3 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id，关联 T_CreditSpecial 表 WorkflowId 字段 |
| 4 | `CommentContent` | text | YES |  |  |  | 评论内容 |
| 5 | `CommentEnclosureId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 评论附件Id |
| 6 | `DingTalks` | varchar(1024) | YES |  |  |  | 发送钉钉json数据 |
| 7 | `CreateTime` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 8 | `CreateById` | int(11) | NO |  |  |  | 创建人 |
| 9 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_UserLabelAuth`

- 说明：用户标签权限表
- 约 45 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `UserId` | int(11) | NO |  |  |  | Erp用户Id |
| 3 | `VendorLabels` | varchar(32) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台；存放格式：,1,2, |
| 4 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 5 | `CreateById` | int(11) | NO |  |  |  | Erp创建人Id |
| 6 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 7 | `ModifyById` | int(11) | YES |  |  |  | Erp修改人Id |
| 8 | `RemarkIn` | varchar(256) | YES |  |  |  | 对内备注 |
| 9 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_UserTreeNode`

- 说明：用户组织树节点(按NodeId UUID做数据可见范围)
- 约 126 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `idx_tree_parent` | 普通 | `ParentNodeId`, `IsDelete` |
| `idx_tree_path` | 普通 | `NodePath`, `IsDelete` |
| `idx_tree_user` | 普通 | `UserId`, `IsDelete` |
| `PRIMARY` | 主键 | `SysNo` |
| `uk_node_delete` | 唯一 | `NodeId`, `IsDelete`, `IsDeletedMark` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增主键（内部使用） |
| 2 | `NodeId` | char(36) | NO | MUL |  |  | 节点Id(UUID) |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 EnumSrmVendorLabel，如1广告线 |
| 4 | `UserId` | int(11) | NO | MUL |  |  | Erp用户Id |
| 5 | `ParentNodeId` | char(36) | NO | MUL | 00000000-0000-0000-0000-000000000000 |  | 父节点NodeId(UUID)，空UUID表示1级根 |
| 6 | `NodePath` | varchar(2048) | NO | MUL |  |  | 物化路径（UserId链），如 /101/205/306/ |
| 7 | `Sort` | int(11) | NO |  | 0 |  | 同级排序，越大越靠前 |
| 8 | `Remark` | varchar(256) | YES |  |  |  | 备注 |
| 9 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 10 | `CreateById` | int(11) | NO |  |  |  | Erp创建人Id |
| 11 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 12 | `ModifyById` | int(11) | YES |  |  |  | Erp修改人Id |
| 13 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0正常 1删除 |
| 14 | `IsDeletedMark` | datetime(3) | NO |  | 0001-01-01 00:00:00.000 |  | 删除时间标记，未删除为默认值 |

### `Srm_VendorContract`

- 说明：供应商合同表
- 约 804 行
- 主键：`VendorContractId`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_VendorContract_BusinessId` | 普通 | `BusinessId` |
| `IX_Srm_VendorContract_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_VendorContract_InstanceId` | 普通 | `InstanceId` |
| `IX_Srm_VendorContract_VendorContractId` | 普通 | `VendorContractId` |
| `IX_Srm_VendorContract_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `IX_Srm_VendorContract_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `VendorContractId` |
| `SysNo` | 唯一 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | UNI |  | auto_increment | 自增长，唯一 |
| 2 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id |
| 3 | `VendorContractId` | char(36) | NO | PRI |  |  | 合同Id |
| 4 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 5 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 6 | `VendorBusinessUnit` | int(11) | NO |  |  |  | 供应商所属业务，0 默认,1 广告线-仅供应商，2 广告线-仅服务商，4 广告线-供应商和服务商 |
| 7 | `StartTime` | datetime | NO |  |  |  | 合同开始时间，格式：yyyy-MM-dd 00:00:00 |
| 8 | `EndTime` | datetime | NO |  |  |  | 合同结束时间，格式：yyyy-MM-dd 23:59:59 |
| 9 | `BelongingCostLine` | varchar(64) | NO |  |  |  | 所属成本线 |
| 10 | `CostBelongingDepartment` | varchar(256) | NO |  |  |  | 费用所属部门 |
| 11 | `IsSupplementaryAgreement` | varchar(4) | NO |  |  |  | 是否为补充协议：是/否 |
| 12 | `FileType` | varchar(256) | NO |  |  |  | 文件类型 |
| 13 | `NameOfOurCompany` | varchar(1024) | NO |  |  |  | 我方单位名称，多个，存放json格式 |
| 14 | `ImportanceOfContract` | varchar(32) | NO |  |  |  | 合同重要性 |
| 15 | `WhetherWeSealFirst` | varchar(4) | NO |  |  |  | 我方是否先盖章：是/否 |
| 16 | `UseContractTemplateWhere` | varchar(8) | NO |  |  |  | 使用哪方合同模板：我方/对方 |
| 17 | `IsCanModify` | varchar(4) | NO |  |  |  | 是否可修改：是/否 |
| 18 | `FileName` | varchar(512) | NO |  |  |  | 文件名称 |
| 19 | `FileNum` | varchar(16) | NO |  |  |  | 文件份数 |
| 20 | `Remarks` | varchar(512) | NO |  |  |  | 备注 |
| 21 | `AuditStatus` | int(11) | NO |  | 0 |  | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回 |
| 22 | `AuditTime` | datetime(3) | YES |  |  |  | 审核时间 |
| 23 | `InstanceId` | varchar(64) | NO | MUL |  |  | 实例Id |
| 24 | `BusinessId` | varchar(64) | NO | MUL |  |  | 审批编号 |
| 25 | `DingDingApproveUrl` | varchar(1024) | YES |  |  |  | 钉钉审批的url |
| 26 | `DingTalkJsonData` | longtext | YES |  |  |  | 存放当前钉钉表单完整的原始json格式数据 |
| 27 | `DingTalkFileJsonData` | longtext | YES |  |  |  | 从DingTalkJsonData中抽离表单的文件、附件二次处理的集合json格式数据 |
| 28 | `RemarksIn` | varchar(512) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 29 | `CreateTime` | datetime(3) | NO | MUL |  |  | 合同创建时间/钉钉发起审批时间 |
| 30 | `CreateById` | int(11) | NO |  |  |  | 创建人Id，ERP用户Id |
| 31 | `StaffId` | int(11) | NO |  | 0 |  | 钉钉用户Id |
| 32 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 33 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 34 | `RowCreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 数据行创建时间 |
| 35 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_VendorEvaluateInvite`

- 说明：评价邀请记录表
- 约 37 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |
| `u_InviteId` | 唯一 | `InviteId` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `InviteId` | char(36) | NO | UNI |  |  | 邀请ID |
| 3 | `EvaluateType` | int(11) | NO |  | 0 |  | 评价类型 1 服务评价  2 财务评价 |
| 4 | `Comment` | varchar(256) | YES |  |  |  | 问候 |
| 5 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 6 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 7 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_VendorEvaluateInviteItem`

- 说明：评价邀请记录表明细
- 约 551 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |
| `u_InviteId` | 普通 | `InviteId` |
| `VendorId_VendorLabel` | 普通 | `VendorId`, `VendorLabel` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `InviteId` | char(36) | NO | MUL |  |  | SrmVendorEvaluateInvite.InviteId |
| 3 | `EvaluateType` | int(11) | NO |  | 0 |  | 评价类型 1 服务评价  2 财务评价 |
| 4 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商ID |
| 5 | `VendorLabel` | int(10) | NO |  |  |  | 标签 |
| 6 | `Status` | int(10) | NO |  | 0 |  | 状态 0 待评价  1已评价 -1覆盖 |
| 7 | `BeInviteUserId` | int(11) | NO |  | 0 |  | 被邀请人 |
| 8 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 9 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 10 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_VendorEvaluateRecord`

- 说明：供应商评价记录
- 约 590 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |
| `u_RecordId` | 唯一 | `RecordId` |
| `VendorId_VendorLabel` | 普通 | `VendorId`, `VendorLabel` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商ID |
| 3 | `VendorLabel` | int(10) | NO |  |  |  | 标签 |
| 4 | `EvaluateType` | int(10) | YES |  | 0 |  | 评价类型 1 服务评价  2 财务评价 |
| 5 | `IndustryServiceTypeId` | int(10) | NO |  |  |  | 行业ID |
| 6 | `RuleId` | char(36) | NO |  |  |  | 规则ID |
| 7 | `RecordId` | char(36) | NO | UNI |  |  | 记录ID |
| 8 | `Score` | decimal(10,2) | NO |  |  |  | 得分 |
| 9 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 10 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |

### `Srm_VendorEvaluateRecordData`

- 说明：供应商评价记录明细
- 约 2151 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `PRIMARY` | 主键 | `SysNo` |
| `u_RecordId` | 普通 | `RecordId` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `RecordId` | char(36) | NO | MUL |  |  | SrmVendorEvaluateRecord.RecordId |
| 3 | `Dimension` | varchar(128) | NO |  |  |  | SrmIndustryServiceTypeEvaluateDimensions.Dimension(维度) |
| 4 | `Weight` | decimal(10,2) | NO |  |  |  | 权重（%） |
| 5 | `Selected` | varchar(256) | NO |  |  |  | SrmIndustryServiceTypeEvaluateDimensions.LevelA-B(结果) |
| 6 | `Level` | int(10) | NO |  |  |  | 等级1-5 |
| 7 | `Score` | decimal(10,2) | NO |  |  |  | 得分 |
| 8 | `CreateTime` | datetime(3) | NO |  | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 9 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |

### `Srm_VendorInfo`

- 说明：供应商基本信息表
- 约 2183 行
- 主键：`VendorId`, `VendorLabel`, `IsDelete`, `IsDelete_Mark`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_VendorInfo_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_VendorInfo_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `PRIMARY` | 主键 | `VendorId`, `VendorLabel`, `IsDelete`, `IsDelete_Mark` |
| `SysNo` | 唯一 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | UNI |  | auto_increment | 自增长，唯一 |
| 2 | `VendorId` | varchar(64) | NO | PRI |  |  | 供应商Id，唯一 |
| 3 | `CrCode` | varchar(32) | NO |  |  |  | 统一信用代码 |
| 4 | `VendorName` | varchar(256) | NO |  |  |  | 供应商名称 |
| 5 | `VendorNameShort` | varchar(128) | NO |  |  |  | 供应商名称简称 |
| 6 | `VendorLabel` | int(11) | NO | PRI |  |  | 标签 1 广告线；2 本地生活线；4 数娱线；6 电商线；8 中后台; 10 创节业务; |
| 7 | `VendorBusinessUnit` | int(11) | NO |  |  |  | 供应商所属业务，0 默认,1 广告线-仅供应商，2 广告线-仅服务商，4 广告线-供应商和服务商 |
| 8 | `IndustryServiceTypes` | varchar(32) | NO |  |  |  | 行业及服务类型，存放格式：,1,2, |
| 9 | `RegisteredAddress` | varchar(512) | NO |  |  |  | 注册地址 |
| 10 | `OfficeAddress` | varchar(512) | NO |  |  |  | 办公地址 |
| 11 | `EnterpriseEmail` | varchar(128) | NO |  |  |  | 企业邮箱 |
| 12 | `CompanyOfficeNumber` | varchar(16) | NO |  |  |  | 公司电话 |
| 13 | `FaxNumber` | varchar(16) | NO |  |  |  | 传真号码 |
| 14 | `CompanyWebsite` | varchar(256) | NO |  |  |  | 公司网址 |
| 15 | `Department` | varchar(32) | NO |  |  |  | 所属部门 |
| 16 | `OwnerId` | int(11) | NO |  |  |  | 所属人，ERP会员Id |
| 17 | `PayDateDay` | int(11) | NO |  | 0 |  | 应付款日期，VendorBusinessUnit = 2/4 服务商时，每月的X日，范围：1-31，默认：25 |
| 18 | `CompanyIds` | varchar(64) | NO |  |  |  | 所属账套 存放格式：,1,2, |
| 19 | `IsEnable` | int(11) | NO |  | 0 |  | 是否启用 0 禁用 1 启用 |
| 20 | `WorkflowId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 评级成功后的工作流Id，关联 Srm_RatingQuality 表 WorkflowId，只有在已经评级时才会写入值，有值时一定是已评级 |
| 21 | `RemarksIn` | varchar(256) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 22 | `CreateTime` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 23 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 24 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 25 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 26 | `IsDelete` | bit(1) | NO | PRI | b'0' |  | 是否删除 0 正常  1 删除 |
| 27 | `IsDelete_Mark` | datetime(3) | NO | PRI | 0001-01-01 00:00:00.000 |  | 删除标记 |

### `Srm_VendorOperationRecord`

- 说明：供应商操作记录表
- 约 1865 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_VendorOperationRecord_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_VendorOperationRecord_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `OperationRecordType` | int(11) | NO |  |  |  | 操作记录类型 1 开启供应商；2 禁用供应商；4 修改所属人；6 修改供应商名称；100 其他，备注内容 |
| 3 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 4 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 5 | `BusinessId` | varchar(64) | NO |  |  |  | 关联的业务Id |
| 6 | `Remarks` | varchar(256) | YES |  |  |  | 用户填写备注信息 |
| 7 | `Content` | varchar(256) | YES |  |  |  | 操作内容 |
| 8 | `EnclosureId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 操作凭证Id |
| 9 | `RemarksIn` | varchar(256) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 10 | `CreateTime` | datetime(3) | YES | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 11 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 12 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_VendorPaymentInfo`

- 说明：供应商付款
- 约 256 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `fk_BankAccountId` | 普通 | `BankAccountId` |
| `fk_BelongingCostLine` | 普通 | `BelongingCostLine`, `CostBelongingDepartment`, `Department` |
| `fk_CreateById` | 普通 | `CreateById` |
| `fk_CreateTime` | 普通 | `CreateTime` |
| `fk_VendorContractId` | 普通 | `VendorContractId` |
| `fk_VendorId` | 普通 | `VendorId` |
| `fk_WorkflowId` | 普通 | `WorkflowId` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增长，唯一 |
| 2 | `WorkflowId` | char(36) | NO | MUL |  |  | 工作流Id |
| 3 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 4 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 5 | `VendorContractId` | char(36) | NO | MUL |  |  | 合同Id |
| 6 | `VendorPolicyId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 政策ID |
| 7 | `BelongingCostLine` | int(10) | NO | MUL |  |  | 所属成本线 T_CascadeData.SysNo |
| 8 | `PaymentNature` | int(10) | NO |  |  |  | 付款性质 1境内付款 2境外付款 |
| 9 | `CostBelongingDepartment` | int(10) | NO |  | 0 |  | 费用所属部门 T_CascadeData.SysNo |
| 10 | `Department` | int(10) | NO |  | 0 |  | 部门 T_CascadeData.SysNo |
| 11 | `CostBelongingCompany` | varchar(128) | NO |  |  |  | 费用所属公司 |
| 12 | `AmountCategory` | varchar(32) | NO |  |  |  | 费用类别 |
| 13 | `AssetNumber` | varchar(128) | NO |  |  |  | 固定资产编号 |
| 14 | `AmountReason` | varchar(256) | NO |  |  |  | 付款事由 |
| 15 | `AmountTotal` | decimal(18,4) | NO |  |  |  | 付款总额 |
| 16 | `Currency` | int(10) | NO |  |  |  | 币种 1人民币 2美金 3港币 4新加坡币 |
| 17 | `BankAccount` | varchar(128) | NO |  |  |  | 开户行 |
| 18 | `BankAccountId` | varchar(128) | NO | MUL |  |  | 银行账号 |
| 19 | `PaymentType` | int(10) | NO |  | 0 |  | 付款方式 1转账 2其他 |
| 20 | `HasInvoice` | bit(1) | YES |  | b'0' |  | 现是否有发票 |
| 21 | `InvoiceFile` | char(36) | YES |  | 00000000-0000-0000-0000-000000000000 |  | 发票扫描件 |
| 22 | `IsNeedCallBack` | bit(1) | YES |  | b'0' |  | 是否需要回单 |
| 23 | `Remarks` | varchar(512) | YES |  |  |  | 备注 |
| 24 | `Attachment` | char(36) | YES |  | 00000000-0000-0000-0000-000000000000 |  | 附件 |
| 25 | `AuditStatus` | int(11) | NO |  | 0 |  | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回 |
| 26 | `AuditTime` | datetime(3) | YES |  |  |  | 审核时间 |
| 27 | `RemarksIn` | varchar(512) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 28 | `BusinessId` | varchar(128) | NO |  |  |  | 钉钉审批编号 |
| 29 | `CreateTime` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 30 | `CreateById` | int(11) | NO | MUL |  |  | 创建人Id，ERP用户Id |
| 31 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 32 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 33 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_VendorPolicy`

- 说明：供应商政策表
- 约 1815 行
- 主键：`VendorPolicyId`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_VendorPolicy_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_VendorPolicy_VendorIdVendorLabelIndustryServiceType` | 普通 | `VendorId`, `VendorLabel`, `IndustryServiceType` |
| `IX_Srm_VendorPolicy_VendorIdVendorLabelSkuId` | 普通 | `VendorId`, `VendorLabel`, `SkuId` |
| `IX_Srm_VendorPolicy_VendorPolicyId` | 普通 | `VendorPolicyId` |
| `IX_Srm_VendorPolicy_version_modified_at` | 普通 | `version_modified_at` |
| `PRIMARY` | 主键 | `VendorPolicyId` |
| `SysNo` | 唯一 | `SysNo` |
| `WorkFlowId` | 普通 | `WorkFlowId` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | UNI |  | auto_increment | 自增长，唯一 |
| 2 | `WorkFlowId` | char(36) | NO | MUL | 00000000-0000-0000-0000-000000000000 |  | 审批流ID,单个审批流 |
| 3 | `VendorPolicyId` | char(36) | NO | PRI |  |  | 政策Id |
| 4 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 5 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 6 | `VendorBusinessUnit` | int(11) | NO |  |  |  | 供应商所属业务，0 默认,1 广告线-仅供应商，2 广告线-仅服务商，4 广告线-供应商和服务商 |
| 7 | `SkuId` | varchar(64) | NO |  |  |  | SkuId |
| 8 | `IndustryServiceType` | int(11) | NO |  |  |  | 行业及服务类型 |
| 9 | `IndustryServiceTypeName` | varchar(128) | YES |  |  |  | 行业类型名称 |
| 10 | `PolicyContent` | varchar(512) | NO |  |  |  | 政策信息 |
| 11 | `PurchasePlans` | text | YES |  |  |  | 采购计划 |
| 12 | `ProcurementPlanName` | varchar(256) | NO |  |  |  | erp采购计划名称 |
| 13 | `BudgetAndSettlementContent` | text | YES |  |  |  | 预算&结算内容 |
| 14 | `PolicyName` | varchar(512) | NO |  |  |  | 政策名称 |
| 15 | `CooperationMode` | int(11) | NO |  | 0 |  | 合作模式:单次合作=1,年度框架合作=2 |
| 16 | `StartTime` | datetime | NO |  |  |  | 政策开始时间，格式：yyyy-MM-dd 00:00:00 |
| 17 | `EndTime` | datetime | NO |  |  |  | 政策结束时间，格式：yyyy-MM-dd 23:59:59 |
| 18 | `EndTimeApply` | datetime | NO |  |  |  | 政策申请时的结束时间，格式：yyyy-MM-dd 23:59:59 |
| 19 | `ServiceTarget` | int(11) | NO |  |  |  | 服务对象 1 为我司服务；2 为客户服务 |
| 20 | `CustomerServiceJsonData` | mediumtext | YES |  |  |  | 为客户服务内容，json格式，ServiceTarget = 2 时，有数据 |
| 21 | `SettlementId` | int(11) | NO |  |  |  | 结算方式 100 客户预付；200 一次性付款(按账期结算)；300 分期结算(按账期结算) |
| 22 | `PlannedDisbursementRatio` | varchar(255) | YES |  |  |  | 计划出款比例 |
| 23 | `ReceivableDay` | int(11) | NO |  | 0 |  | 账期，单位：天 |
| 24 | `PurchaseBudgetAmount` | decimal(18,4) | NO |  | 0.0000 |  | 采购预算金额，单位：万元 |
| 25 | `Remarks` | varchar(512) | YES |  |  |  | 用户备注 |
| 26 | `EnclosureId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 附件组Id |
| 27 | `PolicyStatus` | int(11) | NO |  | 0 |  | 政策状态 0 正常 1 标记失效 2 被新政策替换（老政策不可用了） 4 被新政策覆盖（更改老政策的结束时间，老政策历史可用） 6 被新政策覆盖（不更改老政策数据，新老政策开始结束时间一模一样，老政策彻底不可用） |
| 28 | `AuditStatus` | int(11) | NO |  | 0 |  | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回 |
| 29 | `AuditTime` | datetime(3) | YES |  |  |  | 审核时间 |
| 30 | `PolicyCoverId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 当前政策被覆盖的政策Id |
| 31 | `RemarksIn` | varchar(512) | YES |  |  |  | 对内备注信息，不可对用户展示 |
| 32 | `RatingWorkFlowId` | char(36) | NO |  | 00000000-0000-0000-0000-000000000000 |  | 申请政策时的供应商评级信息，对应Srm_RatingQuality的WorkFlowId |
| 33 | `VendorTypes` | varchar(32) | NO |  |  |  | 申请政策时的供应商类型，存放格式：,1,2, |
| 34 | `CreateTime` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) |  | 创建时间 |
| 35 | `CreateById` | int(11) | NO |  |  |  | 创建人Id，ERP用户Id |
| 36 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 37 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id，ERP用户Id |
| 38 | `version_modified_at` | datetime(3) | NO | MUL | CURRENT_TIMESTAMP(3) | on update CURRENT_TIMESTAMP(3) | 数据变更记录标识 |
| 39 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_VendorWhite`

- 说明：供应商白名单表
- 约 65 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `IX_Srm_VendorWhite_CreateTime` | 普通 | `CreateTime` |
| `IX_Srm_VendorWhite_VendorIdVendorLabel` | 普通 | `VendorId`, `VendorLabel` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) | NO | PRI |  | auto_increment | 自增、主键 |
| 2 | `VendorId` | varchar(64) | NO | MUL |  |  | 供应商Id |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 4 | `Remarks` | varchar(512) | YES |  |  |  | 备注信息 |
| 5 | `CreateTime` | datetime(3) | YES | MUL | CURRENT_TIMESTAMP(3) |  |  |
| 6 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 7 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 8 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 9 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

### `Srm_Vendor_Draft`

- 说明：供应商草稿表
- 约 167 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `idx_buss_id` | 普通 | `business`, `create_id` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) unsigned | NO | PRI |  | auto_increment | 主键，自增 |
| 2 | `business` | varchar(64) | NO | MUL |  |  | 业务类型 |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 4 | `json_data` | text | NO |  |  |  | 数据 |
| 5 | `create_id` | int(11) | NO |  |  |  | 创建人ID |
| 6 | `create_time` | datetime | NO |  | CURRENT_TIMESTAMP |  | 创建人时间 |
| 7 | `modify_time` | datetime | YES |  |  | on update CURRENT_TIMESTAMP | 修改人时间 |
| 8 | `is_delete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常 1 删除 |

### `Srm_Vendor_Main_Info`

- 说明：供应商基本信息主体表
- 约 2198 行
- 主键：`SysNo`

索引：

| 索引 | 类型 | 列（按顺序） |
| --- | --- | --- |
| `idx_vendorId` | 唯一 | `vendorId`, `VendorLabel` |
| `PRIMARY` | 主键 | `SysNo` |

| # | 字段 | 类型 | 空 | 键 | 默认值 | 额外 | 说明 |
| ---: | --- | --- | --- | --- | --- | --- | --- |
| 1 | `SysNo` | int(11) unsigned | NO | PRI |  | auto_increment | 主键，自增 |
| 2 | `vendorId` | varchar(64) | NO | MUL |  |  | 供应商Id，唯一 |
| 3 | `VendorLabel` | int(11) | NO |  |  |  | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台 |
| 4 | `VendorTypes` | varchar(32) | NO |  |  |  | 供应商类型 1 运营服务商；2 媒体方；4 媒体代理商；6 其他 存放格式：,1,2, |
| 5 | `Collaborators` | varchar(256) | NO |  |  |  | 合作主体 存放格式：,1,2, |
| 6 | `main_business` | varchar(512) | NO |  |  |  | 主营业务 |
| 7 | `coop_business` | varchar(512) | NO |  |  |  | 合作业务 |
| 8 | `company_advantage` | varchar(500) | NO |  |  |  | 公司优势描述 |
| 9 | `speed` | varchar(500) | NO |  |  |  | 反应速度承诺 |
| 10 | `quality` | varchar(500) | NO |  |  |  | 质量承诺 |
| 11 | `video_team_person` | varchar(128) | NO |  |  |  | 视频团队人数 |
| 12 | `video_team_structure` | varchar(128) | NO |  |  |  | 视频团队结构 |
| 13 | `video_weekcount` | varchar(128) | NO |  |  |  | 视频周数量 |
| 14 | `custom_business` | varchar(128) | NO |  |  |  | 客户行业类型 |
| 15 | `operate_person` | varchar(128) | NO |  |  |  | 优化运营人数 |
| 16 | `skilled_medium` | varchar(128) | NO |  |  |  | 擅长媒体 |
| 17 | `skilled_business` | varchar(128) | NO |  |  |  | 擅长行业 |
| 18 | `disire` | varchar(128) | NO |  |  |  | 期望合作 |
| 19 | `license_enclosureid` | char(36) | YES |  |  |  | 营业执照扫描件（彩色扫描件加盖公章） |
| 20 | `corporate_representative_enclosureid` | char(36) | YES |  |  |  | 法定代表人身份证（彩色扫描件加盖公章） |
| 21 | `industry_enclosureid` | char(36) | YES |  |  |  | 行业相关资质证书（如代理资质证书、广告经营许可等） |
| 22 | `case_enclosureid` | char(36) | YES |  |  |  | 项目案例（合同、发票、水单、验收情况说明） |
| 23 | `company_introduction_enclosureid` | char(36) | YES |  |  |  | 企业介绍（实地拜访报告：规模、行业、体量等介绍） |
| 24 | `other_desc` | text | YES |  |  |  | 其他补充说明（文字) |
| 25 | `other_enclosureid` | char(36) | YES |  |  |  | 其他补充说明（附件） |
| 26 | `CreateTime` | datetime(3) | YES |  | CURRENT_TIMESTAMP(3) |  |  |
| 27 | `CreateById` | int(11) | NO |  |  |  | 创建人Id |
| 28 | `ModifyTime` | datetime(3) | YES |  |  | on update CURRENT_TIMESTAMP(3) | 修改时间 |
| 29 | `ModifyById` | int(11) | YES |  |  |  | 修改人Id |
| 30 | `IsDelete` | bit(1) | NO |  | b'0' |  | 是否删除 0 正常  1 删除 |

