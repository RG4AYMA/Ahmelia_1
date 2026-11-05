package capability

// BuiltinProgrammingCapabilities is the canonical set of programming
// production capabilities currently exposed by the live runtime.
func BuiltinProgrammingCapabilities() []Capability {
	capabilities := []Capability{
		{ID: "programming.read_source", Domain: DomainLocal, Action: "read_source", Description: "leer código fuente de un proyecto", Lane: LaneProduction, Permission: PermissionAutomatic},
		{ID: "programming.create_file", Domain: DomainLocal, Action: "create_file", Description: "crear archivos de código en un proyecto", Lane: LaneProduction, Permission: PermissionAskFirst},
		{ID: "programming.modify_code", Domain: DomainLocal, Action: "modify_code", Description: "modificar código existente", Lane: LaneProduction, Permission: PermissionAskFirst},
		{ID: "programming.build", Domain: DomainLocal, Action: "build", Description: "compilar un proyecto de software", Lane: LaneProduction, Permission: PermissionAskFirst},
		{ID: "programming.run_tests", Domain: DomainLocal, Action: "run_tests", Description: "ejecutar tests de un proyecto", Lane: LaneProduction, Permission: PermissionAskFirst},
		{ID: "programming.debug", Domain: DomainLocal, Action: "debug", Description: "analizar y corregir errores de código", Lane: LaneProduction, Permission: PermissionAskFirst},
	}
	for i := range capabilities {
		capabilities[i].SkillID = "software.development"
	}
	return capabilities
}
