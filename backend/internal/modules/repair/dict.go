package repair

var statusLabels = map[string]string{
	StatusOngoing:  "维修中",
	StatusFinished: "已完成",
}

var resultLabels = map[string]string{
	ResultFixed:        "已修复",
	ResultPendingParts: "待配件",
	ResultObserving:    "观察中",
	ResultUnfixable:    "无法修复",
}

// ResultMeta 描述一个维修结果对应的责任方与维修性质, 用于归集与统计口径。
type ResultMeta struct {
	Result       string `json:"result"`
	Label        string `json:"label"`
	LiableParty  string `json:"liable_party"`
	RepairNature string `json:"repair_nature"`
}

// resultMetas 是维修结果 -> 责任方 / 维修性质 的映射。
var resultMetas = map[string]ResultMeta{
	ResultFixed:        {Result: ResultFixed, Label: "已修复", LiableParty: "维修班组", RepairNature: "修复性维修"},
	ResultPendingParts: {Result: ResultPendingParts, Label: "待配件", LiableParty: "物资供应部门", RepairNature: "待料暂停"},
	ResultObserving:    {Result: ResultObserving, Label: "观察中", LiableParty: "运行监测班组", RepairNature: "观察跟踪"},
	ResultUnfixable:    {Result: ResultUnfixable, Label: "无法修复", LiableParty: "设施产权单位", RepairNature: "报废待更新"},
}

// StatusLabel 返回维修状态的中文名称。
func StatusLabel(status string) string {
	if label, ok := statusLabels[status]; ok {
		return label
	}
	return status
}

// ResultLabel 返回维修结果的中文名称。
func ResultLabel(result string) string {
	if label, ok := resultLabels[result]; ok {
		return label
	}
	return result
}

// ResultMetaOf 返回指定维修结果的元数据, 未登记时 ok 为 false。
func ResultMetaOf(result string) (ResultMeta, bool) {
	meta, ok := resultMetas[result]
	return meta, ok
}

// ResultMetas 按 Results() 的顺序返回全部维修结果元数据。
func ResultMetas() []ResultMeta {
	metas := make([]ResultMeta, 0, len(resultMetas))
	for _, result := range Results() {
		if meta, ok := resultMetas[result]; ok {
			metas = append(metas, meta)
		}
	}
	return metas
}
