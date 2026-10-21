# SRM 数据字典

给模型写只读 SQL 用。来源是业务库 `crm` 里表名以 `Srm` 开头的表（区分大小写），采集自 `information_schema`。
没有物理外键。约行数是 InnoDB 估算，不是精确 COUNT。本文件只有结构，没有业务数据。
规模：33 张表，512 个字段。

把本文件配成会话的数据字典后，系统提示会带上规则和表目录。字段用 `dbconn` 的 schema 动作并指定 table 读取，不要一次拉取全文。未配置字典时 schema 改为实时读取业务库。写 SQL 时以本文件的表名、列名、关联为准。

## 写 SQL 前先看

- 只写单条只读 SELECT。表名原样使用，区分大小写。
- 有 `IsDelete` 的表，查当前数据加 `IsDelete = 0`。`bit(1)` 用 0/1 比较：0 正常，1 删除。
- `Srm_Vendor_Draft` 的删除列是 `is_delete`。`Srm_IndustryServiceTypeEvaluateRules`、`Srm_IndustryServiceTypeEvaluateDimensions`、`Srm_VendorEvaluateRecord`、`Srm_VendorEvaluateRecordData` 没有软删列。
- `IsDelete_Mark`（组织树表是 `IsDeletedMark`）未删除时为 `0001-01-01`，删除后写入删除时间，用来让同一业务键可以多次软删。查当前数据只要 `IsDelete = 0`。
- `SysNo` 是各表自己的自增号。禁止拿 `SysNo` 跨表关联。两个例外：`Srm_IndustryServiceType.SysNo` 是行业类型 Id；`Srm_RatingTemplateHistory.TemplateNumber` 指向 `Srm_RatingTemplate.SysNo`。
- `OwnerId`、`CreateById`、`ModifyById`、`UserId`、`StaffId`、`create_id`、`RowCreateById`、`BeInviteUserId` 是 ERP 人员 Id，人员表不在本字典。姓名只出现在以 `_v` 结尾的快照列。
- 查某个供应商时同时带 `VendorId` 和 `VendorLabel`。只按名称或只按 VendorId 会把不同标签的数据串在一起。
- `VendorLabel` 取值以 `Srm_VendorInfo` 的注释为准：1 广告线，2 本地生活，4 数娱线，6 电商线，8 中后台，10 创节业务。其他表注释常漏写 10，不要再发明取值。
- 列注释里的枚举是唯一合法取值。注释没写的状态不要补。
- 逗号包裹列表（`,1,2,`）按字符串存，匹配用 `LIKE '%,值,%'`。涉及列：`VendorLabels`、`IndustryServiceTypes`、`CompanyIds`、`VendorTypes`、`Collaborators`。
- `char(36)` 默认 `00000000-0000-0000-0000-000000000000` 表示空，JOIN 前应排除。
- 以 `_v` 结尾的列、以及 `*Name` 展示列，是冗余快照。展示用它们，关联用对应的 Id 列。
- `RemarksIn`、`RemarkIn` 是对内备注，不要返回给用户。
- `T_CascadeData`、ERP 用户、钉钉组织不在这些表里。成本线、部门、人员只取本表已有的 Id 和名称快照。
- 列名大小写不一致，必须按下面写：`Srm_Vendor_Main_Info.vendorId`，`Srm_VendorPolicy.WorkFlowId`，`Srm_Vendor_Draft` 用蛇形列名。



## 先选表

- 供应商档案、名称、信用代码、是否启用、所属人：`Srm_VendorInfo`
- 主体资料、供应商类型、资质附件：`Srm_Vendor_Main_Info`
- 未落档的草稿：`Srm_Vendor_Draft`
- 联系人：`Srm_ContactInfo`；收款账户：`Srm_PayInfo`；白名单：`Srm_VendorWhite`
- 合同：`Srm_VendorContract`；政策：`Srm_VendorPolicy`
- 业务付款申请：`Srm_BusinessPaymentInfo`；供应商付款：`Srm_VendorPaymentInfo`
- 质量评级结果：`Srm_RatingQuality`，指标明细：`Srm_RatingQualityDetail`
- 评级调整：`Srm_RatingQualityAdjust` 与 `Srm_RatingQualityDetailAdjust`
- 评级模板当前值：`Srm_RatingTemplate` → `Srm_RatingTarget` → `Srm_RatingTargetConfig`；历史加 `History`
- 服务/财务评价：`Srm_VendorEvaluateRecord` 与 `Srm_VendorEvaluateRecordData`；邀请：`Srm_VendorEvaluateInvite` 与 `Srm_VendorEvaluateInviteItem`
- 行业类型：`Srm_IndustryServiceType`；评价规则：`Srm_IndustryServiceTypeEvaluateRules` 与 `Srm_IndustryServiceTypeEvaluateDimensions`
- 特批：`Srm_SpecialApprovalReport` 与 `Srm_SpecialApprovalReport_Comment`
- 谁能看哪些标签：`Srm_UserLabelAuth`；组织可见范围：`Srm_UserTreeNode`
- 供应商启停、改名、改所属人：`Srm_VendorOperationRecord`



## 关联

供应商主线。两边都有这两列时一起等值连接，并各自加上软删条件：

```sql
a.VendorId = b.VendorId AND a.VendorLabel = b.VendorLabel
```

`Srm_Vendor_Main_Info` 写成 `m.vendorId = v.VendorId AND m.VendorLabel = v.VendorLabel`。

其他关联只在下面这些列上做，不要因为列名相同就连接 `VersionNumber`、`BusinessId`、`CreateById`、`SysNo`。

- `Srm_VendorContract.VendorContractId` = `Srm_BusinessPaymentInfo.VendorContractId` = `Srm_VendorPaymentInfo.VendorContractId`
- `Srm_VendorPolicy.VendorPolicyId` = `Srm_BusinessPaymentInfo.VendorPolicyId` = `Srm_VendorPaymentInfo.VendorPolicyId`
- `Srm_RatingQuality.WorkflowId` = `Srm_RatingQualityDetail.WorkflowId`
- `Srm_VendorInfo.WorkflowId` = `Srm_RatingQuality.WorkflowId`（仅已评级后有值）
- `Srm_VendorPolicy.RatingWorkFlowId` = `Srm_RatingQuality.WorkflowId`
- `Srm_RatingQualityAdjust` 与 `Srm_RatingQualityDetailAdjust`：`WorkflowFatherId` 与 `WorkflowSonId` 成对相等
- `Srm_VendorEvaluateInvite.InviteId` = `Srm_VendorEvaluateInviteItem.InviteId`
- `Srm_VendorEvaluateRecord.RecordId` = `Srm_VendorEvaluateRecordData.RecordId`
- `Srm_IndustryServiceTypeEvaluateRules.RuleId` = `Srm_IndustryServiceTypeEvaluateDimensions.RuleId` = `Srm_VendorEvaluateRecord.RuleId`
- `Srm_IndustryServiceType.SysNo` = `Srm_IndustryServiceTypeConfig.IndustryServiceTypeId` = `Srm_IndustryServiceTypeEvaluateRules.IndustryServiceTypeId` = `Srm_VendorEvaluateRecord.IndustryServiceTypeId`
- `Srm_RatingTemplate.TemplateId` = `Srm_RatingTarget.TemplateId` = `Srm_RatingTargetConfig.TemplateId`；历史表再加同一个 `VersionNumber`
- `Srm_RatingTemplate.SysNo` = `Srm_RatingTemplateHistory.TemplateNumber`
- `Srm_SpecialApprovalReport.WorkflowId` = `Srm_SpecialApprovalReport_Comment.WorkflowId`
- `Srm_UserTreeNode.ParentNodeId` = 另一行的 `NodeId`；全零 UUID 是根节点

付款单、合同、特批、评级各自有 `WorkflowId`，只在上面点名的表之间互相关联，不要跨业务把所有 `WorkflowId` 连成一张图。

`BankAccountId` 只存在于两张付款表，`Srm_PayInfo` 没有这一列。

## 常用查询骨架

```sql
-- 当前供应商
SELECT VendorId, VendorLabel, VendorName, CrCode, IsEnable
FROM Srm_VendorInfo
WHERE IsDelete = 0

-- 供应商 + 联系人
SELECT v.VendorId, v.VendorLabel, v.VendorName, c.ContactName, c.Mobile
FROM Srm_VendorInfo v
JOIN Srm_ContactInfo c
  ON c.VendorId = v.VendorId AND c.VendorLabel = v.VendorLabel AND c.IsDelete = 0
WHERE v.IsDelete = 0

-- 供应商 + 主体资料
SELECT v.VendorName, m.VendorTypes, m.main_business
FROM Srm_VendorInfo v
JOIN Srm_Vendor_Main_Info m
  ON m.vendorId = v.VendorId AND m.VendorLabel = v.VendorLabel AND m.IsDelete = 0
WHERE v.IsDelete = 0

-- 供应商当前评级
SELECT v.VendorName, q.Legal_Rating, q.Legal_ComScore, q.RatingStatus
FROM Srm_VendorInfo v
JOIN Srm_RatingQuality q
  ON q.VendorId = v.VendorId AND q.VendorLabel = v.VendorLabel AND q.IsDelete = 0
WHERE v.IsDelete = 0
```



## 表目录



### 供应商主数据

- `Srm_VendorInfo` 供应商基本信息表 | 主键 VendorId+VendorLabel+IsDelete+IsDelete_Mark | 当前行 IsDelete=0 | 约 2184 行
- `Srm_Vendor_Main_Info` 供应商基本信息主体表 | 主键 SysNo | 当前行 IsDelete=0 | 约 2199 行
- `Srm_Vendor_Draft` 供应商草稿表 | 主键 SysNo | 当前行 is_delete=0 | 约 167 行
- `Srm_ContactInfo` 供应商联系人信息表 | 主键 SysNo | 当前行 IsDelete=0 | 约 2007 行
- `Srm_PayInfo` 供应商收款信息表 | 主键 SysNo | 当前行 IsDelete=0 | 约 1990 行
- `Srm_VendorWhite` 供应商白名单表 | 主键 SysNo | 当前行 IsDelete=0 | 约 65 行
- `Srm_VendorOperationRecord` 供应商操作记录表 | 主键 SysNo | 当前行 IsDelete=0 | 约 1869 行
- `Srm_UserLabelAuth` 用户标签权限表 | 主键 SysNo | 当前行 IsDelete=0 | 约 45 行
- `Srm_UserTreeNode` 用户组织树节点(按NodeId UUID做数据可见范围) | 主键 SysNo | 当前行 IsDelete=0 | 约 126 行



### 行业与评价

- `Srm_IndustryServiceType` 供应商行业及服务类型 | 主键 SysNo | 当前行 IsDelete=0 | 约 72 行
- `Srm_IndustryServiceTypeConfig` 供应商行业及服务类型配置表 | 主键 SysNo | 当前行 IsDelete=0 | 约 113 行
- `Srm_IndustryServiceTypeEvaluateRules` 评价规则 | 主键 SysNo | 当前行 无软删列，不要加 IsDelete | 约 113 行
- `Srm_IndustryServiceTypeEvaluateDimensions` 评价维度 | 主键 SysNo | 当前行 无软删列，不要加 IsDelete | 约 436 行
- `Srm_VendorEvaluateInvite` 评价邀请记录表 | 主键 SysNo | 当前行 IsDelete=0 | 约 37 行
- `Srm_VendorEvaluateInviteItem` 评价邀请记录表明细 | 主键 SysNo | 当前行 IsDelete=0 | 约 551 行
- `Srm_VendorEvaluateRecord` 供应商评价记录 | 主键 SysNo | 当前行 无软删列，不要加 IsDelete | 约 590 行
- `Srm_VendorEvaluateRecordData` 供应商评价记录明细 | 主键 SysNo | 当前行 无软删列，不要加 IsDelete | 约 2151 行



### 合同、政策、付款

- `Srm_VendorContract` 供应商合同表 | 主键 VendorContractId | 当前行 IsDelete=0 | 约 805 行
- `Srm_VendorPolicy` 供应商政策表 | 主键 VendorPolicyId | 当前行 IsDelete=0 | 约 1841 行
- `Srm_BusinessPaymentInfo` 业务付款流程v2.0,v3.0 | 主键 SysNo | 当前行 IsDelete=0 | 约 2390 行
- `Srm_VendorPaymentInfo` 供应商付款 | 主键 SysNo | 当前行 IsDelete=0 | 约 256 行



### 质量评级

- `Srm_RatingTemplate` 供应商评级模版表 | 主键 SysNo | 当前行 IsDelete=0 | 约 11 行
- `Srm_RatingTemplateHistory` 供应商评级模版历史表 | 主键 SysNo | 当前行 IsDelete=0 | 约 43 行
- `Srm_RatingTarget` 供应商评级模版指标表 | 主键 SysNo | 当前行 IsDelete=0 | 约 103 行
- `Srm_RatingTargetHistory` 供应商评级模版指标历史表 | 主键 SysNo | 当前行 IsDelete=0 | 约 397 行
- `Srm_RatingTargetConfig` 供应商评级模版指标配置表 | 主键 SysNo | 当前行 IsDelete=0 | 约 466 行
- `Srm_RatingTargetConfigHistory` 供应商评级模版指标配置历史表 | 主键 SysNo | 当前行 IsDelete=0 | 约 1760 行
- `Srm_RatingQuality` 供应商质量评级表 | 主键 SysNo | 当前行 IsDelete=0 | 约 1377 行
- `Srm_RatingQualityDetail` 供应商质量评级明细表 | 主键 SysNo | 当前行 IsDelete=0 | 约 10611 行
- `Srm_RatingQualityAdjust` 供应商质量评级调整表 | 主键 SysNo | 当前行 IsDelete=0 | 约 1791 行
- `Srm_RatingQualityDetailAdjust` 供应商质量评级明细调整表 | 主键 SysNo | 当前行 IsDelete=0 | 约 11907 行



### 特批

- `Srm_SpecialApprovalReport` 特批报备 | 主键 SysNo | 当前行 IsDelete=0 | 约 102 行
- `Srm_SpecialApprovalReport_Comment` 特批评论表 | 主键 SysNo | 当前行 IsDelete=0 | 约 19 行



## 字段

每列一行：`列名 类型 空约束 键 自增 | 注释`。NN 非空，NULL 可空，PK 主键，UQ 唯一，IDX 普通索引，AI 自增。联合索引的列顺序写在该表的索引行。

## 供应商主数据



### Srm_VendorInfo

当前供应商档案。一行是某供应商标签下的一条记录，含已软删版本。查当前数据用 IsDelete=0。同一 VendorId 可有多个 VendorLabel，过滤供应商时两个条件一起带。

约 2184 行。当前数据：IsDelete=0。

索引：PK PRIMARY(VendorId,VendorLabel,IsDelete,IsDelete_Mark); UQ SysNo(SysNo); IDX IX_Srm_VendorInfo_CreateTime(CreateTime); IDX IX_Srm_VendorInfo_VendorIdVendorLabel(VendorId,VendorLabel)

- SysNo int(11) NN UQ AI | 自增长，唯一
- VendorId varchar(64) NN PK | 供应商Id，唯一
- CrCode varchar(32) NN | 统一信用代码
- VendorName varchar(256) NN | 供应商名称
- VendorNameShort varchar(128) NN | 供应商名称简称
- VendorLabel int(11) NN PK | 标签 1 广告线；2 本地生活线；4 数娱线；6 电商线；8 中后台; 10 创节业务;
- VendorBusinessUnit int(11) NN | 供应商所属业务，0 默认,1 广告线-仅供应商，2 广告线-仅服务商，4 广告线-供应商和服务商
- IndustryServiceTypes varchar(32) NN | 行业及服务类型，存放格式：,1,2,
- RegisteredAddress varchar(512) NN | 注册地址
- OfficeAddress varchar(512) NN | 办公地址
- EnterpriseEmail varchar(128) NN | 企业邮箱
- CompanyOfficeNumber varchar(16) NN | 公司电话
- FaxNumber varchar(16) NN | 传真号码
- CompanyWebsite varchar(256) NN | 公司网址
- Department varchar(32) NN | 所属部门
- OwnerId int(11) NN | 所属人，ERP会员Id
- PayDateDay int(11) NN | 应付款日期，VendorBusinessUnit = 2/4 服务商时，每月的X日，范围：1-31，默认：25
- CompanyIds varchar(64) NN | 所属账套 存放格式：,1,2,
- IsEnable int(11) NN | 是否启用 0 禁用 1 启用
- WorkflowId char(36) NN | 评级成功后的工作流Id，关联 Srm_RatingQuality 表 WorkflowId，只有在已经评级时才会写入值，有值时一定是已评级
- RemarksIn varchar(256) NULL | 对内备注信息，不可对用户展示
- CreateTime datetime(3) NN IDX | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN PK | 是否删除 0 正常  1 删除
- IsDelete_Mark datetime(3) NN PK | 删除标记



### Srm_Vendor_Main_Info

供应商主体扩展资料，一行对应一个 vendorId + VendorLabel。关联 VendorInfo 时列名是小写 vendorId。

约 2199 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); UQ idx_vendorId(vendorId,VendorLabel)

- SysNo int(11) unsigned NN PK AI | 主键，自增
- vendorId varchar(64) NN IDX | 供应商Id，唯一
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台；10创节业务；
- VendorTypes varchar(32) NN | 供应商类型 1 运营服务商；2 媒体方；4 媒体代理商；6 其他 存放格式：,1,2,
- Collaborators varchar(256) NN | 合作主体 存放格式：,1,2,
- main_business varchar(512) NN | 主营业务
- coop_business varchar(512) NN | 合作业务
- company_advantage varchar(500) NN | 公司优势描述
- speed varchar(500) NN | 反应速度承诺
- quality varchar(500) NN | 质量承诺
- video_team_person varchar(128) NN | 视频团队人数
- video_team_structure varchar(128) NN | 视频团队结构
- video_weekcount varchar(128) NN | 视频周数量
- custom_business varchar(128) NN | 客户行业类型
- operate_person varchar(128) NN | 优化运营人数
- skilled_medium varchar(128) NN | 擅长媒体
- skilled_business varchar(128) NN | 擅长行业
- disire varchar(128) NN | 期望合作
- license_enclosureid char(36) NULL | 营业执照扫描件（彩色扫描件加盖公章）
- corporate_representative_enclosureid char(36) NULL | 法定代表人身份证（彩色扫描件加盖公章）
- industry_enclosureid char(36) NULL | 行业相关资质证书（如代理资质证书、广告经营许可等）
- case_enclosureid char(36) NULL | 项目案例（合同、发票、水单、验收情况说明）
- company_introduction_enclosureid char(36) NULL | 企业介绍（实地拜访报告：规模、行业、体量等介绍）
- other_desc text NULL | 其他补充说明（文字)
- other_enclosureid char(36) NULL | 其他补充说明（附件）
- CreateTime datetime(3) NULL | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_Vendor_Draft

未提交的供应商草稿，正文在 json_data。删除列是 is_delete，不是 IsDelete。没有 VendorId。

约 167 行。当前数据：is_delete=0。

索引：PK PRIMARY(SysNo); IDX idx_buss_id(business,create_id)

- SysNo int(11) unsigned NN PK AI | 主键，自增
- business varchar(64) NN IDX | 业务类型
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- json_data text NN | 数据
- create_id int(11) NN | 创建人ID
- create_time datetime NN | 创建人时间
- modify_time datetime NULL | 修改人时间
- is_delete bit(1) NN | 是否删除 0 正常 1 删除



### Srm_ContactInfo

供应商联系人，可多行。用 VendorId + VendorLabel 连到 VendorInfo。

约 2007 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_ContactInfo_CreateTime(CreateTime); IDX IX_Srm_ContactInfo_VendorIdVendorLabel(VendorId,VendorLabel)

- SysNo int(11) NN PK AI | 自增、主键
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- ContactName varchar(32) NN | 联系人姓名
- Position varchar(32) NN | 职位
- Email varchar(64) NN | 邮箱
- Mobile varchar(16) NN | 手机号
- CreateTime datetime(3) NULL IDX | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_PayInfo

供应商收款账户。银行账号列是 BankAccount，没有 BankAccountId，不能拿付款单的 BankAccountId 来连这张表。

约 1990 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_PayInfo_CreateTime(CreateTime); IDX IX_Srm_PayInfo_VendorIdVendorLabel(VendorId,VendorLabel)

- SysNo int(11) NN PK AI | 自增、主键
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- BankName varchar(256) NN | 银行名称
- BankAccount varchar(64) NULL | 银行账户
- CreateTime datetime(3) NULL IDX | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_VendorWhite

供应商白名单。在名单内且 IsDelete=0 表示白名单有效。

约 65 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_VendorWhite_CreateTime(CreateTime); IDX IX_Srm_VendorWhite_VendorIdVendorLabel(VendorId,VendorLabel)

- SysNo int(11) NN PK AI | 自增、主键
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- Remarks varchar(512) NULL | 备注信息
- CreateTime datetime(3) NULL IDX | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_VendorOperationRecord

供应商启停、改名、改所属人等操作流水。BusinessId 是当时关联的业务号，不是统一外键。

约 1869 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_VendorOperationRecord_CreateTime(CreateTime); IDX IX_Srm_VendorOperationRecord_VendorIdVendorLabel(VendorId,VendorLabel)

- SysNo int(11) NN PK AI | 自增、主键
- OperationRecordType int(11) NN | 操作记录类型 1 开启供应商；2 禁用供应商；4 修改所属人；6 修改供应商名称；100 其他，备注内容
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- BusinessId varchar(64) NN | 关联的业务Id
- Remarks varchar(256) NULL | 用户填写备注信息
- Content varchar(256) NULL | 操作内容
- EnclosureId char(36) NN | 操作凭证Id
- RemarksIn varchar(256) NULL | 对内备注信息，不可对用户展示
- CreateTime datetime(3) NULL IDX | 创建时间
- CreateById int(11) NN | 创建人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_UserLabelAuth

ERP 用户可看哪些供应商标签。VendorLabels 是逗号包裹列表，不是单个 VendorLabel。

约 45 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- UserId int(11) NN | Erp用户Id
- VendorLabels varchar(32) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台；存放格式：,1,2,
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | Erp创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | Erp修改人Id
- RemarkIn varchar(256) NULL | 对内备注
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_UserTreeNode

按组织树节点限制数据可见范围。删除标记列名是 IsDeletedMark，不是 IsDelete_Mark。NodePath 形如 /101/205/306/。

约 126 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); UQ uk_node_delete(NodeId,IsDelete,IsDeletedMark); IDX idx_tree_parent(ParentNodeId,IsDelete); IDX idx_tree_path(NodePath,IsDelete); IDX idx_tree_user(UserId,IsDelete)

- SysNo int(11) NN PK AI | 自增主键（内部使用）
- NodeId char(36) NN IDX | 节点Id(UUID)
- VendorLabel int(11) NN | 标签 EnumSrmVendorLabel，如1广告线
- UserId int(11) NN IDX | Erp用户Id
- ParentNodeId char(36) NN IDX | 父节点NodeId(UUID)，空UUID表示1级根
- NodePath varchar(2048) NN IDX | 物化路径（UserId链），如 /101/205/306/
- Sort int(11) NN | 同级排序，越大越靠前
- Remark varchar(256) NULL | 备注
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | Erp创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | Erp修改人Id
- IsDelete bit(1) NN | 是否删除 0正常 1删除
- IsDeletedMark datetime(3) NN | 删除时间标记，未删除为默认值



## 行业与评价



### Srm_IndustryServiceType

行业及服务类型字典。这里的 SysNo 是类型 Id，其他表的 IndustryServiceTypeId 指向它。这是少数允许用 SysNo 做关联的表。

约 72 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 行业及服务类型Id，自增，主键
- IndustryServiceTypeName varchar(64) NN | 行业及服务类型名称
- Remarks varchar(256) NULL | 用户备注信息
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id，ERP用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_IndustryServiceTypeConfig

标签与行业类型的配置。注释写的 Srm_VendorIndustryServiceType 不存在，实际连 Srm_IndustryServiceType.SysNo。

约 113 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- IndustryServiceTypeId int(11) NN | 行业及服务类型Id，关联 Srm_VendorIndustryServiceType 表 SysNo 字段
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- Remarks varchar(256) NULL | 用户备注信息
- RemarksIn varchar(256) NULL | 对内备注信息，不可对用户展示
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id，ERP用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_IndustryServiceTypeEvaluateRules

评价规则。RuleId 唯一。没有 IsDelete。

约 113 行。当前数据：无软删列，不要加 IsDelete。

索引：PK PRIMARY(SysNo); UQ u_RuleId(RuleId); IDX IndustryServiceTypeId_VendorLabel(IndustryServiceTypeId,VendorLabel)

- SysNo int(11) NN PK AI | 自增长，唯一
- EvaluateType int(10) NULL | 评价类型 1 服务评价  2 财务评价
- VendorLabel int(10) NN | 标签
- IndustryServiceTypeId int(10) NN IDX | 行业ID
- RuleId char(36) NN UQ | 规则ID
- Description varchar(256) NULL | 描述
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id



### Srm_IndustryServiceTypeEvaluateDimensions

某条规则下的评分维度。用 RuleId 连规则表。没有 IsDelete。

约 436 行。当前数据：无软删列，不要加 IsDelete。

索引：PK PRIMARY(SysNo); IDX u_RuleId(RuleId)

- SysNo int(11) NN PK AI | 自增长，唯一
- RuleId char(36) NN IDX | 规则ID
- Dimension varchar(128) NN | 维度
- LevelA varchar(256) NN | 等级1（极差）
- LevelB varchar(256) NN | 等级2（较差）
- LevelC varchar(256) NN | 等级3（合格）
- LevelD varchar(256) NN | 等级4（优秀）
- LevelE varchar(256) NN | 等级5（卓越）
- Weight decimal(10,2) NN | 权重（%）



### Srm_VendorEvaluateInvite

一次评价邀请。InviteId 唯一。

约 37 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); UQ u_InviteId(InviteId)

- SysNo int(11) NN PK AI | 自增长，唯一
- InviteId char(36) NN UQ | 邀请ID
- EvaluateType int(11) NN | 评价类型 1 服务评价  2 财务评价
- Comment varchar(256) NULL | 问候
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_VendorEvaluateInviteItem

邀请里的供应商明细。InviteId 连邀请表，VendorId + VendorLabel 连供应商。

约 551 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX VendorId_VendorLabel(VendorId,VendorLabel); IDX u_InviteId(InviteId)

- SysNo int(11) NN PK AI | 自增长，唯一
- InviteId char(36) NN IDX | SrmVendorEvaluateInvite.InviteId
- EvaluateType int(11) NN | 评价类型 1 服务评价  2 财务评价
- VendorId varchar(64) NN IDX | 供应商ID
- VendorLabel int(10) NN | 标签
- Status int(10) NN | 状态 0 待评价  1已评价 -1覆盖
- BeInviteUserId int(11) NN | 被邀请人
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_VendorEvaluateRecord

一次供应商评价得分。RecordId 唯一。没有 IsDelete。

约 590 行。当前数据：无软删列，不要加 IsDelete。

索引：PK PRIMARY(SysNo); UQ u_RecordId(RecordId); IDX VendorId_VendorLabel(VendorId,VendorLabel)

- SysNo int(11) NN PK AI | 自增长，唯一
- VendorId varchar(64) NN IDX | 供应商ID
- VendorLabel int(10) NN | 标签
- EvaluateType int(10) NULL | 评价类型 1 服务评价  2 财务评价
- IndustryServiceTypeId int(10) NN | 行业ID
- RuleId char(36) NN | 规则ID
- RecordId char(36) NN UQ | 记录ID
- Score decimal(10,2) NN | 得分
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id



### Srm_VendorEvaluateRecordData

评价得分的维度明细。RecordId 连评价记录。没有 IsDelete。

约 2151 行。当前数据：无软删列，不要加 IsDelete。

索引：PK PRIMARY(SysNo); IDX u_RecordId(RecordId)

- SysNo int(11) NN PK AI | 自增长，唯一
- RecordId char(36) NN IDX | SrmVendorEvaluateRecord.RecordId
- Dimension varchar(128) NN | SrmIndustryServiceTypeEvaluateDimensions.Dimension(维度)
- Weight decimal(10,2) NN | 权重（%）
- Selected varchar(256) NN | SrmIndustryServiceTypeEvaluateDimensions.LevelA-B(结果)
- Level int(10) NN | 等级1-5
- Score decimal(10,2) NN | 得分
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id



## 合同、政策、付款



### Srm_VendorContract

供应商合同。主键是 VendorContractId，不是 SysNo。全零 UUID 表示未填。

约 805 行。当前数据：IsDelete=0。

索引：PK PRIMARY(VendorContractId); UQ SysNo(SysNo); IDX IX_Srm_VendorContract_BusinessId(BusinessId); IDX IX_Srm_VendorContract_CreateTime(CreateTime); IDX IX_Srm_VendorContract_InstanceId(InstanceId); IDX IX_Srm_VendorContract_VendorContractId(VendorContractId); IDX IX_Srm_VendorContract_VendorIdVendorLabel(VendorId,VendorLabel); IDX IX_Srm_VendorContract_WorkflowId(WorkflowId)

- SysNo int(11) NN UQ AI | 自增长，唯一
- WorkflowId char(36) NN IDX | 工作流Id
- VendorContractId char(36) NN PK | 合同Id
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- VendorBusinessUnit int(11) NN | 供应商所属业务，0 默认,1 广告线-仅供应商，2 广告线-仅服务商，4 广告线-供应商和服务商
- StartTime datetime NN | 合同开始时间，格式：yyyy-MM-dd 00:00:00
- EndTime datetime NN | 合同结束时间，格式：yyyy-MM-dd 23:59:59
- BelongingCostLine varchar(64) NN | 所属成本线
- CostBelongingDepartment varchar(256) NN | 费用所属部门
- IsSupplementaryAgreement varchar(4) NN | 是否为补充协议：是/否
- FileType varchar(256) NN | 文件类型
- NameOfOurCompany varchar(1024) NN | 我方单位名称，多个，存放json格式
- ImportanceOfContract varchar(32) NN | 合同重要性
- WhetherWeSealFirst varchar(4) NN | 我方是否先盖章：是/否
- UseContractTemplateWhere varchar(8) NN | 使用哪方合同模板：我方/对方
- IsCanModify varchar(4) NN | 是否可修改：是/否
- FileName varchar(512) NN | 文件名称
- FileNum varchar(16) NN | 文件份数
- Remarks varchar(512) NN | 备注
- AuditStatus int(11) NN | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回
- AuditTime datetime(3) NULL | 审核时间
- InstanceId varchar(64) NN IDX | 实例Id
- BusinessId varchar(64) NN IDX | 审批编号
- DingDingApproveUrl varchar(1024) NULL | 钉钉审批的url
- DingTalkJsonData longtext NULL | 存放当前钉钉表单完整的原始json格式数据
- DingTalkFileJsonData longtext NULL | 从DingTalkJsonData中抽离表单的文件、附件二次处理的集合json格式数据
- RemarksIn varchar(512) NULL | 对内备注信息，不可对用户展示
- CreateTime datetime(3) NN IDX | 合同创建时间/钉钉发起审批时间
- CreateById int(11) NN | 创建人Id，ERP用户Id
- StaffId int(11) NN | 钉钉用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- RowCreateTime datetime(3) NN | 数据行创建时间
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_VendorPolicy

供应商政策。主键是 VendorPolicyId。审批流列名是 WorkFlowId（F 大写），评级回写列是 RatingWorkFlowId。

约 1841 行。当前数据：IsDelete=0。

索引：PK PRIMARY(VendorPolicyId); UQ SysNo(SysNo); IDX IX_Srm_VendorPolicy_CreateTime(CreateTime); IDX IX_Srm_VendorPolicy_VendorIdVendorLabelIndustryServiceType(VendorId,VendorLabel,IndustryServiceType); IDX IX_Srm_VendorPolicy_VendorIdVendorLabelSkuId(VendorId,VendorLabel,SkuId); IDX IX_Srm_VendorPolicy_VendorPolicyId(VendorPolicyId); IDX IX_Srm_VendorPolicy_version_modified_at(version_modified_at); IDX WorkFlowId(WorkFlowId)

- SysNo int(11) NN UQ AI | 自增长，唯一
- WorkFlowId char(36) NN IDX | 审批流ID,单个审批流
- VendorPolicyId char(36) NN PK | 政策Id
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- VendorBusinessUnit int(11) NN | 供应商所属业务，0 默认,1 广告线-仅供应商，2 广告线-仅服务商，4 广告线-供应商和服务商
- SkuId varchar(64) NN | SkuId
- IndustryServiceType int(11) NN | 行业及服务类型
- IndustryServiceTypeName varchar(128) NULL | 行业类型名称
- PolicyContent varchar(512) NN | 政策信息
- PurchasePlans text NULL | 采购计划
- ProcurementPlanName varchar(256) NN | erp采购计划名称
- BudgetAndSettlementContent text NULL | 预算&结算内容
- PolicyName varchar(512) NN | 政策名称
- CooperationMode int(11) NN | 合作模式:单次合作=1,年度框架合作=2
- StartTime datetime NN | 政策开始时间，格式：yyyy-MM-dd 00:00:00
- EndTime datetime NN | 政策结束时间，格式：yyyy-MM-dd 23:59:59
- EndTimeApply datetime NN | 政策申请时的结束时间，格式：yyyy-MM-dd 23:59:59
- ServiceTarget int(11) NN | 服务对象 1 为我司服务；2 为客户服务
- CustomerServiceJsonData mediumtext NULL | 为客户服务内容，json格式，ServiceTarget = 2 时，有数据
- SettlementId int(11) NN | 结算方式 100 客户预付；200 一次性付款(按账期结算)；300 分期结算(按账期结算)
- PlannedDisbursementRatio varchar(255) NULL | 计划出款比例
- ReceivableDay int(11) NN | 账期，单位：天
- PurchaseBudgetAmount decimal(18,4) NN | 采购预算金额，单位：万元
- Remarks varchar(512) NULL | 用户备注
- EnclosureId char(36) NN | 附件组Id
- PolicyStatus int(11) NN | 政策状态 0 正常 1 标记失效 2 被新政策替换（老政策不可用了） 4 被新政策覆盖（更改老政策的结束时间，老政策历史可用） 6 被新政策覆盖（不更改老政策数据，新老政策开始结束时间一模一样，老政策彻底不可用）
- AuditStatus int(11) NN | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回
- AuditTime datetime(3) NULL | 审核时间
- PolicyCoverId char(36) NN | 当前政策被覆盖的政策Id
- RemarksIn varchar(512) NULL | 对内备注信息，不可对用户展示
- RatingWorkFlowId char(36) NN | 申请政策时的供应商评级信息，对应Srm_RatingQuality的WorkFlowId
- VendorTypes varchar(32) NN | 申请政策时的供应商类型，存放格式：,1,2,
- CreateTime datetime(3) NN IDX | 创建时间
- CreateById int(11) NN | 创建人Id，ERP用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- version_modified_at datetime(3) NN IDX | 数据变更记录标识
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_BusinessPaymentInfo

业务付款流程 v2/v3。一行一笔付款申请。VendorContractId、VendorPolicyId 为全零 UUID 时表示没挂合同或政策。

约 2390 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX fk_BankAccountId(BankAccountId); IDX fk_BelongingCostLine(BelongingCostLine,CostBelongingDepartment,Department); IDX fk_CreateById(CreateById); IDX fk_CreateTime(CreateTime); IDX fk_InstanceId(InstanceId); IDX fk_ProcessCode(ProcessCode); IDX fk_VendorContractId(VendorContractId); IDX fk_VendorId(VendorId); IDX fk_WorkflowId(WorkflowId)

- SysNo int(11) NN PK AI | 自增长，唯一
- WorkflowId char(36) NN IDX | 工作流Id
- ApplyType int(11) NN | 审批类型:300 业务付款流程v2.0;310 业务付款流程v3.0
- ProcessCode varchar(64) NN IDX | 审批模板的唯一码
- InstanceId varchar(64) NN IDX | 实例Id
- BusinessId varchar(128) NN | 钉钉审批编号
- VendorId varchar(64) NN IDX | 供应商Id
- VendorName varchar(128) NN | 供应商名称
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- VendorContractId char(36) NN IDX | 合同Id
- VendorPolicyId char(36) NN | 政策ID
- BelongingCostLine int(10) NN IDX | 所属成本线 T_CascadeData.SysNo，这里直接写 0
- BelongingCostLineName_v varchar(256) NN | 所属成本线名称
- PaymentNature int(10) NN | 付款性质Id 1境内付款 2境外付款，这里直接写 0
- PaymentNatureName_v varchar(256) NN | 付款性质名称
- CostBelongingDepartment int(10) NN | 费用所属部门Id T_CascadeData.SysNo，这里直接写 0
- CostBelongingDepartmentName_v varchar(256) NN | 费用所属部门名称
- Department int(10) NN | 部门Id T_CascadeData.SysNo，这里直接写 0
- DepartmentName_v varchar(256) NN | 部门名称
- CostBelongingCompany varchar(128) NN | 费用所属公司
- AmountCategory varchar(32) NN | 费用类别
- AssetNumber varchar(128) NN | 固定资产编号
- AmountReason varchar(256) NN | 付款事由
- AmountTotal decimal(18,4) NN | 付款总额
- Currency int(10) NN | 币种 1人民币 2美金 3港币 4新加坡币 99 其他
- CurrencyName_v varchar(32) NN | 币种名称
- BankAccount varchar(128) NN | 开户行
- BankAccountId varchar(128) NN IDX | 银行账号
- PaymentType int(10) NN | 付款方式Id 1转账 2其他，这里直接写 0
- PaymentTypeName_v varchar(64) NN | 付款方式名称
- HasInvoice bit(1) NN | 现是否有发票
- InvoiceFile char(36) NN | 发票扫描件
- IsNeedCallBack bit(1) NULL | 是否需要回单
- Remarks varchar(512) NULL | 备注
- Attachment char(36) NULL | 附件
- AuditStatus int(11) NN | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回
- AuditTime datetime(3) NULL | 审核时间
- RemarksIn varchar(512) NULL | 对内备注信息，不可对用户展示
- DingDingApproveUrl_v varchar(1024) NN | 钉钉审批的url
- RowCreateTime datetime(3) NN | 数据行创建时间
- CreateTime datetime(3) NN IDX | 创建时间
- CreateById int(11) NN IDX | 创建人Id
- CreateByName_v varchar(128) NN | 创建人姓名
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_VendorPaymentInfo

供应商付款。结构与业务付款相近，但没有流程模板、实例和名称快照列。

约 256 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX fk_BankAccountId(BankAccountId); IDX fk_BelongingCostLine(BelongingCostLine,CostBelongingDepartment,Department); IDX fk_CreateById(CreateById); IDX fk_CreateTime(CreateTime); IDX fk_VendorContractId(VendorContractId); IDX fk_VendorId(VendorId); IDX fk_WorkflowId(WorkflowId)

- SysNo int(11) NN PK AI | 自增长，唯一
- WorkflowId char(36) NN IDX | 工作流Id
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- VendorContractId char(36) NN IDX | 合同Id
- VendorPolicyId char(36) NN | 政策ID
- BelongingCostLine int(10) NN IDX | 所属成本线 T_CascadeData.SysNo
- PaymentNature int(10) NN | 付款性质 1境内付款 2境外付款
- CostBelongingDepartment int(10) NN | 费用所属部门 T_CascadeData.SysNo
- Department int(10) NN | 部门 T_CascadeData.SysNo
- CostBelongingCompany varchar(128) NN | 费用所属公司
- AmountCategory varchar(32) NN | 费用类别
- AssetNumber varchar(128) NN | 固定资产编号
- AmountReason varchar(256) NN | 付款事由
- AmountTotal decimal(18,4) NN | 付款总额
- Currency int(10) NN | 币种 1人民币 2美金 3港币 4新加坡币
- BankAccount varchar(128) NN | 开户行
- BankAccountId varchar(128) NN IDX | 银行账号
- PaymentType int(10) NN | 付款方式 1转账 2其他
- HasInvoice bit(1) NULL | 现是否有发票
- InvoiceFile char(36) NULL | 发票扫描件
- IsNeedCallBack bit(1) NULL | 是否需要回单
- Remarks varchar(512) NULL | 备注
- Attachment char(36) NULL | 附件
- AuditStatus int(11) NN | 申请审批状态 0 草稿，1 新建/提交审核 2 审核中， 4 审核通过， -1 驳回 -2 申请人撤回
- AuditTime datetime(3) NULL | 审核时间
- RemarksIn varchar(512) NULL | 对内备注信息，不可对用户展示
- BusinessId varchar(128) NN | 钉钉审批编号
- CreateTime datetime(3) NN IDX | 创建时间
- CreateById int(11) NN IDX | 创建人Id，ERP用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



## 质量评级



### Srm_RatingTemplate

当前评级模板。TemplateId 是业务键，SysNo 只在历史表 TemplateNumber 上被引用。VendorLabels 为逗号包裹列表。

约 11 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键、模版编号
- VersionNumber char(64) NULL | 模版版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TemplateName varchar(256) NN | 模版名称
- VendorLabels varchar(32) NN | 模版适用标签，存放格式：,1,2,
- Remarks varchar(256) NULL | 用户备注信息
- Status int(11) NN | 是否启用 0 关闭 1 启用
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id，ERP用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_RatingTemplateHistory

模板历史。TemplateNumber = Srm_RatingTemplate.SysNo。用 VersionNumber + TemplateId 对齐指标历史。

约 43 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- VersionNumber char(64) NULL | 版本号，格式：V2022-06-16 10:37:51
- TemplateNumber int(11) NN | 模版编号，关联 Srm_RatingTemplate 表 SysNo 字段
- TemplateId char(36) NN | 模版Id
- TemplateName varchar(256) NN | 模版名称
- VendorLabels varchar(32) NN | 模版适用标签，存放格式：,1,2,
- Remarks varchar(256) NULL | 用户备注信息
- Status int(11) NN | 1 启用 -1 停用
- CreateTime datetime(3) NN | 创建时间
- CreateById int(11) NN | 创建人Id，ERP用户Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id，ERP用户Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除
- RowCreateTime datetime(3) NN | 数据行创建时间
- RowCreateById int(11) NN | 数据行创建人Id，ERP用户Id



### Srm_RatingTarget

当前模板下的指标。用 TemplateId 连模板，用 TargetId 连指标配置。

约 103 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- VersionNumber char(64) NULL | 版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TargetId char(36) NN | 指标Id
- TargetName varchar(256) NN | 指标名称
- WeightRatio decimal(10,2) NN | 权重，百分比
- IsQccTarget int(11) NN | 是否是企查查指标 0 不是 1 是
- Sort int(11) NN | 排序，数值越小越靠前
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_RatingTargetHistory

指标历史。用 VersionNumber + TemplateId + TargetId 对齐。

约 397 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- VersionNumber char(64) NULL | 版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TargetId char(36) NN | 指标Id
- TargetName varchar(256) NN | 指标名称
- WeightRatio decimal(10,2) NN | 权重，百分比
- IsQccTarget int(11) NN | 是否是企查查指标 0 不是 1 是
- Sort int(11) NN | 排序，数值越小越靠前
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除
- RowCreateTime datetime(3) NN | 数据行创建时间
- RowCreateById int(11) NN | 数据行创建人Id，ERP用户Id



### Srm_RatingTargetConfig

指标下的评分项。用 TemplateId + TargetId 连指标。

约 466 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- VersionNumber char(64) NULL | 版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TargetId char(36) NN | 指标Id
- TargetItemId char(36) NN | 指标项Id
- TargetItemName varchar(128) NN | 指标项
- TargetItemScore int(11) NN | 指标项分数
- Sort int(11) NN | 排序，数值越小越靠前
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_RatingTargetConfigHistory

评分项历史。用 VersionNumber + TemplateId + TargetId + TargetItemId 对齐。

约 1760 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo)

- SysNo int(11) NN PK AI | 自增、主键
- VersionNumber char(64) NULL | 版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TargetId char(36) NN | 指标Id
- TargetItemId char(36) NN | 指标项Id
- TargetItemName varchar(128) NN | 指标项
- TargetItemScore int(11) NN | 指标项分数
- Sort int(11) NN | 排序，数值越小越靠前
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除
- RowCreateTime datetime(3) NN | 数据行创建时间
- RowCreateById int(11) NN | 数据行创建人Id，ERP用户Id



### Srm_RatingQuality

供应商质量评级结果。用 VendorId + VendorLabel 连供应商，用 WorkflowId 连评级明细。VendorInfo.WorkflowId 只在已评级后指向这里。

约 1377 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_RatingQuality_CreateTime(CreateTime); IDX IX_Srm_RatingQuality_VendorIdVendorLabel(VendorId,VendorLabel); IDX IX_Srm_RatingQuality_WorkflowId(WorkflowId)

- SysNo int(11) NN PK AI | 自增、主键
- WorkflowId char(36) NN IDX | 工作流Id
- VersionNumber char(64) NN | 版本号，格式：V2022-06-16 10:37:51
- VendorId varchar(64) NN IDX | 供应商Id
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- AdjustContent varchar(512) NULL | 调整事项
- AdjustContentScore decimal(18,2) NN | 调整事项，评分
- Legal_Rating varchar(2) NULL | 质量评级 A B C
- Legal_ComScore decimal(18,2) NN | 综合评分
- Legal_Remarks varchar(512) NN | 法务备注
- RatingStatus int(11) NN | 评级状态 0 未准入 1 新建 2 准入审批中 4 准入通过，等待评级 6 已准入，已评级  -1 驳回 -2 撤回
- RemarksIn varchar(256) NULL | 对内备注信息，不可对用户展示
- CreateTime datetime(3) NULL IDX | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_RatingQualityDetail

某次评级的指标得分明细。用 WorkflowId 连 Srm_RatingQuality，不要用 SysNo。

约 10611 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_RatingQualityDetail_WorkflowId(WorkflowId)

- SysNo int(11) NN PK AI | 自增、主键
- WorkflowId char(36) NN IDX | 工作流Id
- TemplateVersionNumber char(64) NULL | 模版版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TemplateName varchar(256) NN | 模版名称
- TargetId char(36) NN | 指标Id
- TargetName varchar(256) NN | 指标名称
- TargetWeightRatio decimal(10,2) NN | 指标权重，百分比
- TargetIsQccTarget int(11) NN | 指标是否是企查查指标 0 不是 1 是
- TargetSort int(11) NN | 指标排序，数值越小越靠前
- TargetItemId char(36) NN | 指标项Id
- TargetItemName varchar(128) NN | 指标项
- TargetItemScore int(11) NN | 指标项分数
- CreateTime datetime(3) NULL | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_RatingQualityAdjust

评级调整单。用 WorkflowFatherId 对原评级，WorkflowSonId 对调整后明细。

约 1791 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_RatingQualityAdjust_WorkflowFatherId(WorkflowFatherId); IDX IX_Srm_RatingQualityAdjust_WorkflowSonId(WorkflowSonId)

- SysNo int(11) NN PK AI | 自增、主键
- WorkflowFatherId char(36) NN IDX | 父工作流Id
- WorkflowSonId char(36) NN IDX | 子工作流Id
- AdjustContent varchar(512) NULL | 调整事项
- AdjustContentScore decimal(18,2) NN | 调整事项，评分
- Legal_Rating varchar(2) NULL | 质量评级 A B C
- Legal_ComScore decimal(18,2) NN | 综合评分
- Legal_Remarks varchar(512) NN | 法务备注
- RemarksIn varchar(256) NULL | 对内备注信息，不可对用户展示
- CreateTime datetime(3) NULL | 创建时间
- CreateById int(11) NN | 创建人Id
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_RatingQualityDetailAdjust

评级调整明细。与调整单使用同一对 WorkflowFatherId、WorkflowSonId。

约 11907 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_Srm_RatingQualityDetailAdjust_WorkflowFatherId(WorkflowFatherId); IDX IX_Srm_RatingQualityDetailAdjust_WorkflowSonId(WorkflowSonId)

- SysNo int(11) NN PK AI | 自增、主键
- WorkflowFatherId char(36) NN IDX | 父工作流Id
- WorkflowSonId char(36) NN IDX | 子工作流Id
- TemplateVersionNumber char(64) NULL | 模版版本号，格式：V2022-06-16 10:37:51
- TemplateId char(36) NN | 模版Id
- TemplateName varchar(256) NN | 模版名称
- TargetId char(36) NN | 指标Id
- TargetName varchar(256) NN | 指标名称
- TargetWeightRatio decimal(10,2) NN | 指标权重，百分比
- TargetIsQccTarget int(11) NN | 指标是否是企查查指标 0 不是 1 是
- TargetSort int(11) NN | 指标排序，数值越小越靠前
- TargetItemId char(36) NN | 指标项Id
- TargetItemName varchar(128) NN | 指标项
- TargetItemScore int(11) NN | 指标项分数
- CreateTime datetime(3) NULL | 创建时间
- CreateById int(11) NN | 创建人Id
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



## 特批



### Srm_SpecialApprovalReport

特批报备。用 VendorId + VendorLabel 连供应商。VersionNumber 在这张表上有索引，评级模板的 VersionNumber 不是同一条数据。

约 102 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_T_SrmSpecialApprove_AccountId_VendorLabel(VendorId,VendorLabel); IDX IX_T_SrmSpecialApprove_CreateTime(CreateTime); IDX IX_T_SrmSpecialApprove_VersionNumber(VersionNumber); IDX IX_T_SrmSpecialApprove_WorkflowId(WorkflowId)

- SysNo int(11) NN PK AI | 主键，自增
- WorkflowId char(36) NN IDX | 工作流Id
- VersionNumber varchar(64) NN IDX | 版本号，格式：Vyyyy-MM-dd HH:mm:ss
- VendorId varchar(64) NN IDX | 供应商Id，唯一
- VendorLabel int(11) NN | 标签 1 广告线；2 本地生活；4 数娱线；6 电商线；8 中后台
- AuditStatus int(11) NN | 授信评审委员会审批结果 4 通过 -1 评审不通过
- AuditEnclosureId char(36) NN | 审批附件Id
- OtherEnclosureId char(36) NN | 相关增信文件或实地拜访报告Id
- Remarks text NULL | 备注
- DingTalks varchar(1024) NULL | 发送钉钉json数据
- CreateTime datetime(3) NN IDX | 创建时间
- CreateById int(11) NN | 创建人
- ModifyTime datetime(3) NULL | 修改时间
- ModifyById int(11) NULL | 修改人
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除



### Srm_SpecialApprovalReport_Comment

特批报备评论。用 WorkflowId 连 Srm_SpecialApprovalReport。注释里的 T_CreditSpecial 不在本字典。

约 19 行。当前数据：IsDelete=0。

索引：PK PRIMARY(SysNo); IDX IX_T_SrmSpecialComment_CommentId(CommentId); IDX IX_T_SrmSpecialComment_CreateTime(CreateTime); IDX IX_T_SrmSpecialComment_WorkflowId(WorkflowId)

- SysNo int(11) NN PK AI | 主键，自增
- CommentId char(36) NN IDX | 评论Id
- WorkflowId char(36) NN IDX | 工作流Id，关联 T_CreditSpecial 表 WorkflowId 字段
- CommentContent text NULL | 评论内容
- CommentEnclosureId char(36) NN | 评论附件Id
- DingTalks varchar(1024) NULL | 发送钉钉json数据
- CreateTime datetime(3) NN IDX | 创建时间
- CreateById int(11) NN | 创建人
- IsDelete bit(1) NN | 是否删除 0 正常  1 删除

