package api

type Result interface {
	ResultId() string
	IsSuccess() bool
	Message() string
}

type ResultBase struct {
	Id            string
	Success       bool
	ResultMessage string
}

func NewResultBase(id string, success bool, message string) *ResultBase {
	return &ResultBase{
		Id:            id,
		Success:       success,
		ResultMessage: message,
	}
}

func (r *ResultBase) ResultId() string {
	return r.Id
}

func (r *ResultBase) IsSuccess() bool {
	return r.Success
}

func (r *ResultBase) Message() string {
	return r.ResultMessage
}
