package compose

import "github.com/geocodio/treehouse/internal/instance"

// ResolveInstance walks upward from dir looking for .treehouse.env (see
// instance.FindEnvFileUpwards) and loads it if found. It returns a nil
// *instance.Instance, not an error, when dir is outside any instance —
// callers pass that nil straight to Compose/Exec/Up/Down/Stop/Restart to
// get main-repository passthrough behavior.
func ResolveInstance(dir string) (*instance.Instance, error) {
	envFilePath, found, err := instance.FindEnvFileUpwards(dir)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	return instance.LoadEnvFile(envFilePath)
}
