package api

type Result interface {
	Id() string
	IsSuccess() bool
	Message() string
}

type ResultBase struct {
	ResultId string
	Success  bool
	Message  string
}

func NewResultBase(id string, success bool, message string) *ResultBase {
	return &ResultBase{
		ResultId: id,
		Success:  success,
		Message:  message,
	}
}

func (r *ResultBase) Id() string {
	return r.ResultId
}

func (r *ResultBase) IsSuccess() bool {
	return r.Success
}

func (r *ResultBase) Message() string {
	return r.Message
}
