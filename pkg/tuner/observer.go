package tuner

type Observer interface {
	GetEnvironment() *Environment
}

// abstract class
type BaseObserver struct {
}
