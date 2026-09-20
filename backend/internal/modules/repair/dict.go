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

// 责任方: 不同维修结果对应不同的责任归属。
const (
	LiabilityMaintenanceTeam = "maintenance_team" // 维修班组
	LiabilityMaterialSupply  = "material_supply"  // 物资供应
	LiabilityAssetOwner      = "asset_owner"      // 资产权属方
)

// 维修性质: 不同维修结果对应不同的维修性质。
const (
	NatureFaultRepair     = "fault_repair"     // 故障维修
	NaturePendingMaterial = "pending_material" // 待料缓修
	NatureObserveTrack    = "observe_track"    // 观察跟踪
	NatureRetrofit        = "retrofit"         // 更新改造
)

var liabilityLabels = map[string]string{
	LiabilityMaintenanceTeam: "维修班组",
	LiabilityMaterialSupply:  "物资供应",
	LiabilityAssetOwner:      "资产权属方",
}

var natureLabels = map[string]string{
	NatureFaultRepair:     "故障维修",
	NaturePendingMaterial: "待料缓修",
	NatureObserveTrack:    "观察跟踪",
	NatureRetrofit:        "更新改造",
}

// resultAttribution 定义维修结果到 责任方 / 维修性质 的映射, 是统计归集的统一口径。
var resultAttribution = map[string][2]string{
	ResultFixed:        {LiabilityMaintenanceTeam, NatureFaultRepair},
	ResultPendingParts: {LiabilityMaterialSupply, NaturePendingMaterial},
	ResultObserving:    {LiabilityMaintenanceTeam, NatureObserveTrack},
	ResultUnfixable:    {LiabilityAssetOwner, NatureRetrofit},
}

// Liabilities 返回全部责任方取值。
func Liabilities() []string {
	return []string{LiabilityMaintenanceTeam, LiabilityMaterialSupply, LiabilityAssetOwner}
}

// Natures 返回全部维修性质取值。
func Natures() []string {
	return []string{NatureFaultRepair, NaturePendingMaterial, NatureObserveTrack, NatureRetrofit}
}

// ResultAttribution 返回维修结果对应的责任方与维修性质, 非法结果返回空串。
func ResultAttribution(result string) (liability string, nature string) {
	if pair, ok := resultAttribution[result]; ok {
		return pair[0], pair[1]
	}
	return "", ""
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

// LiabilityLabel 返回责任方的中文名称。
func LiabilityLabel(liability string) string {
	if label, ok := liabilityLabels[liability]; ok {
		return label
	}
	return liability
}

// NatureLabel 返回维修性质的中文名称。
func NatureLabel(nature string) string {
	if label, ok := natureLabels[nature]; ok {
		return label
	}
	return nature
}
