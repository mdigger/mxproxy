package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kr/pretty"
	"github.com/mdigger/apns"
	"github.com/mdigger/rest"
)

func TestGoogle(t *testing.T) {
	gfcmKey := "AAAA0bHpCVQ:APA91bFQ_14Nvgr2q7dOp2WORG8hOoaxpxfSegbc8qgmSq6zrw5RHYpklEU6K55fg7DrhW0Q1ycIe9JzjEtfj-S5soQg5hnIdiYSQKWWM1oIFK5Hpa1aLtOf7_JW6jTP-5LrMY6p7Yge"
	var gfcmMsg = &struct {
		RegistrationIDs []string    `json:"registration_ids,omitempty"`
		Data            interface{} `json:"data,omitempty"`
		TTL             uint16      `json:"time_to_live"`
	}{
		RegistrationIDs: []string{
			"cyK-QJZq8dE:APA91bGzw62nmJ0xbg3L0Nq1Q7HtILp2oK5AZ_iLmM80xCzbBvH-DSlM4F0LEqIl84s3dxT8nWMidD8HQ-uZTCCTD9Hy3REdkbzL-zgvwHBt0stPjOq4i6OLE5RSoK7mQh39esgQ8648",
			"cJSf2PNlwwk:APA91bGhc88UOpk7og0KTHZcyYEWJszhvzUwf_X2JmjYp7BCwZWxxW5WVZVc2FTYsJktmF27UMPNit-aF_9zcfgmB4waYtbqsyd7L3UoTkYKDxsA3AtvudfHD6U3xsVz1Vz3xrUVjlct",
			"ekNgAYO7Cqg:APA91bE8LfIfzeNY2iexHj3OsvebbQyvaQ5TE9O1vRHxAc93lRyLVmSHUisKRCaXSqjhcVqUnDDvXFgpT8HohluLgeHCdw5rYYXZZg4l9aUsqGCJ60Dwj84ureu4-JeXo0iK8Ycmqtsl",
			"ekNgAYO7Cqg:APA91bHoUz1wHepLaB7EnkN-mbP6XMrSuHAQrSrE1IliXSentpxzGTP_jZw6gwg8hG3EOm_D7RLQ_Hg2o7EbQxxtER7ofEChhyeNKAmiqIL_NSThiH7LCEU6IMze-9z2wIcmRcgSSyak",
			"fffffff",
		},
		Data: rest.JSON{
			"time": time.Now().UTC(),
		},
	}
	data, err := json.Marshal(gfcmMsg)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("POST",
		"https://fcm.googleapis.com/fcm/send", bytes.NewReader(data))
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+gfcmKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode == http.StatusOK {
		// TODO: разобрать ошибку
		type googleResponse struct {
			// MulticastID  int64 `json:"multicast_id"`
			Success int `json:"success"`
			Failure int `json:"failure"`
			// CanonicalIDs int `json:"canonical_ids"`
			Results []struct {
				// MessageID      *string `json:"message_id"`
				RegistrationID string `json:"registration_id"`
				Error          string `json:"error"`
			} `json:"results"`
		}
		var gresp = new(googleResponse)
		if err := json.NewDecoder(resp.Body).Decode(gresp); err != nil {
			t.Fatal(err)
		}
		pretty.Println(gresp)
		// проходим по массиву результатов
		for indx, result := range gresp.Results {
			switch result.Error {
			case "":
				// нужно заменить на новый номер
				if result.RegistrationID != "" {
					fmt.Printf("replace %s to %s\n",
						gfcmMsg.RegistrationIDs[indx],
						result.RegistrationID)
				} else {
					fmt.Printf("! OK %s\n",
						gfcmMsg.RegistrationIDs[indx])
				}
			case "Unavailable": // все хорошо или не доступен, попробуйте позже
				fmt.Printf("! OK %s\n",
					gfcmMsg.RegistrationIDs[indx])
			default:
				fmt.Printf("- remove %s\n",
					gfcmMsg.RegistrationIDs[indx])
			}
		}
	} else {
		fmt.Println(resp.Status)
	}
	resp.Body.Close()
}

func TestDevider(t *testing.T) {
	bundleID := "apns:test.bundle"
	devider := strings.IndexByte(bundleID, ':')
	fmt.Printf("%q : %q\n", bundleID[:devider], bundleID[devider+1:])
}

func TestSendPush(t *testing.T) {
	cert, err := apns.LoadCertificate("PushTest.p12", "xopen123")
	if err != nil {
		t.Fatal(err)
	}
	// cinfo := apns.GetCertificateInfo(cert)
	// enc := json.NewEncoder(os.Stdout)
	// enc.SetIndent("", "  ")
	// enc.Encode(cinfo)

	client := apns.New(*cert)
	client.Host = "https://api.development.push.apple.com"
	_, err = client.Push(apns.Notification{
		Token: "7C179108B7BF759DED2D9CBED7969DE6623D34E200E46387E7D713917E0F3EB8",
		Payload: map[string]interface{}{
			"time": time.Now().UTC(),
		},
	})
	if err != nil {
		pretty.Println(err)
		pretty.Println(client)
	}
}

// func TestPush(t *testing.T) {
// 	tokensDBName := appName + ".db"
// 	var err error
// 	storeDB, err = OpenStore(tokensDBName)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	defer storeDB.Close()

// 	userID := fmt.Sprintf("%s:%s", "63022", csta.JID(43884851428118509))
// 	if err := Push(userID, nil); err != nil {
// 		t.Fatal(err)
// 	}
// }
