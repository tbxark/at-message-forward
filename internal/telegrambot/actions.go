package telegrambot

type action struct {
	title    string
	commands []string
	parent   string
}

var actionsByID = map[string]action{
	"status_summary":     {title: "Status Summary", parent: "status", commands: []string{"+CPIN?", "+CSQ", "+CREG?", "+CEREG?", "+COPS?", "+CFUN?"}},
	"signal":             {title: "Signal Quality", parent: "status", commands: []string{"+CSQ"}},
	"registration":       {title: "Network Registration", parent: "status", commands: []string{"+CREG?", "+CEREG?"}},
	"operator":           {title: "Operator", parent: "status", commands: []string{"+COPS?"}},
	"sim":                {title: "SIM Status", parent: "status", commands: []string{"+CPIN?", "+CCID"}},
	"module":             {title: "Module Info", parent: "status", commands: []string{"+CGMI", "+CGMM", "+CGMR", "+CGSN"}},
	"sms_unread":         {title: "Unread SMS", parent: "sms", commands: []string{"+CMGF=1", "+CMGL=\"REC UNREAD\""}},
	"sms_all":            {title: "All SMS", parent: "sms", commands: []string{"+CMGF=1", "+CMGL=\"ALL\""}},
	"sms_storage":        {title: "SMS Storage", parent: "sms", commands: []string{"+CPMS?"}},
	"enable_sms_storage": {title: "Store SMS to SIM", parent: "sms", commands: []string{"+CMGF=1", "+CNMI=2,1,0,0,0"}},
	"function_mode":      {title: "Current Function Mode", parent: "device", commands: []string{"+CFUN?"}},
	"enable_sms_push":    {title: "Re-enable SMS Push", parent: "device", commands: []string{"+CMGF=1", "+CNMI=2,2,0,0,0"}},
	"reset":              {title: "Restart Module", parent: "device", commands: []string{"+RESET"}},
}

func actionForID(id string) (action, bool) {
	act, ok := actionsByID[id]
	return act, ok
}
