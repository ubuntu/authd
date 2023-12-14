package gdm

func init() {
	checkMembersFunc = checkMembersDebug
	validateJSONFunc = validateJSONDebug
}
