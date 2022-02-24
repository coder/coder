package terraform_provider


type EnvironmentVariable struct {
	Name string
	Value string
}

type Config struct {
	AdditionalArgs string
	EnvironmentVariables [] EnvironmentVariable
}