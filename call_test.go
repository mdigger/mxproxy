package main

import (
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"strings"
	"testing"

	"github.com/kr/pretty"
	"github.com/mdigger/csta"
	"github.com/mdigger/rest"
)

func call(c *rest.Context) error {
	// Params описывает параметры, передаваемые в запроса
	type Params struct {
		RingDelay uint8  `xml:"ringdelay,attr" json:"ringDelay" form:"ringDelay"`
		VMDelay   uint8  `xml:"vmdelay,attr" json:"vmDelay" form:"vmDelay"`
		From      string `xml:"address" json:"from" form:"from"`
		To        string `xml:"-" json:"to" form:"to"`
	}
	// инициализируем параметры по умолчанию и разбираем запрос
	var params = &Params{
		RingDelay: 1,
		VMDelay:   30,
	}
	if err := c.Bind(params); err != nil {
		return err
	}

	var cmd = &struct {
		XMLName xml.Name `xml:"iq"`
		Type    string   `xml:"type,attr"`
		ID      string   `xml:"id,attr"`
		Mode    string   `xml:"mode,attr"`
		*Params
	}{Type: "set", ID: "mode", Mode: "remote", Params: params}

	data, err := xml.Marshal(cmd)
	if err != nil {
		return err
	}
	return c.Write(data)
}

func TestPostCall(t *testing.T) {
	mux := new(rest.ServeMux)
	mux.Handle("POST", "/test", call)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/test", "application/json",
		strings.NewReader(`{
	"ringDelay": 2,
	"vmDelay": 28,
	"from": "79031744445",
	"to": "79031744437"
}`))
	if err != nil {
		t.Fatal(err)
	}
	dump, err := httputil.DumpResponse(resp, true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%s\n", dump)
}

func TestCall(t *testing.T) {
	csta.SetLogOutput(os.Stdout)
	csta.SetLogFlags(0)

	client, err := csta.NewClient("89.185.246.134:7778", csta.Login{
		UserName: "dm",
		Password: "78561",
		Type:     "User",
		Platform: "iPhone",
		Version:  "1.0",
		// UserName: "d3test",
		// Password: "981211",
		// Type:     "Server",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	_, err = client.Send(`<iq type="set" id="mode" mode="remote" ringdelay="1" vmdelay="20"><address>79031744445</address></iq>`)
	if err != nil {
		t.Fatal(err)
	}
	id, err := client.Send(`<MakeCall><callingDevice typeOfNumber="deviceID">3095</callingDevice><calledDirectoryNumber>1099</calledDirectoryNumber></MakeCall>`)
	if err != nil {
		t.Fatal(err)
	}

	client.SetWait(MXReadTimeout)
read2:
	resp, err := client.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != id {
		goto read2
	}
	switch resp.Name {
	case "CSTAErrorCode":
		cstaError := new(CSTAError)
		if err := resp.Decode(cstaError); err != nil {
			t.Fatal(err)
		}
		t.Error(cstaError)
	case "MakeCallResponse":
		// <MakeCallResponse>
		// 	<callingDevice>
		// 		<callID>25</callID>
		// 		<deviceID>3095</deviceID>
		// 	</callingDevice>
		// 	<calledDevice>1099</calledDevice>
		// </MakeCallResponse>
		var result = new(struct {
			CallID       uint64 `xml:"callingDevice>callID"`
			DeviceID     string `xml:"callingDevice>deviceID"`
			CalledDevice string `xml:"calledDevice"`
		})
		if err := resp.Decode(result); err != nil {
			t.Fatal(err)
		}
		pretty.Println("callInfo:", result)
	default:
		t.Error("unknown response")
	}

	// client.SetWait(time.Second * 60)
	// for {
	// 	if _, err := client.Receive(); err != nil {
	// 		t.Fatal(err)
	// 	}
	// }
}

func TestParseCSTAError(t *testing.T) {
	// cstaError := []byte(`<CSTAErrorCode><privateErrorCode>URM Denied</privateErrorCode></CSTAErrorCode>`)
	// p := new(struct {
	// 	Error string `xml:",any"`
	// })
	// if err := xml.Unmarshal(cstaError, p); err != nil {
	// 	t.Fatal(err)
	// }
	// pretty.Println(p)

	type callingDevice struct {
		Type string `xml:"typeOfNumber,attr"`
		Ext  string `xml:",chardata"`
	}
	f2 := &struct {
		XMLName       xml.Name      `xml:"http://www.ecma-international.org/standards/ecma-323/csta/ed4 MakeCall"`
		CallingDevice callingDevice `xml:"callingDevice"`
		To            string        `xml:"calledDirectoryNumber"`
	}{
		CallingDevice: callingDevice{
			Type: "deviceID",
			Ext:  "224",
		},
		To: "234234234",
	}

	if err := xml.NewEncoder(os.Stdout).Encode(f2); err != nil {
		t.Fatal(err)
	}
}
