package jwtutils

import (
	"context"
	"fmt"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwt"
)

// The keys and token below are test fixtures only. They live in a _test.go
// file so they are never compiled into the application.

var ExampleTok, _ = jwt.NewBuilder().
	Issuer("my-app").
	Subject("user-123").
	Audience([]string{"my-api"}).
	IssuedAt(time.Now()).
	Expiration(time.Now().Add(1*time.Hour)).
	Claim("custom-claim", "hello-world").
	Build()

var ExampleMyPrivateKey = `{
  "alg": "RS256",
  "d": "Kukq7Hptw51UYCQrfs3RPafpTaQM91mWulmgDi-hwk4sAfNSNCxWVwhBIepe6oVrNhME_tLOCH5xRpLYISxUhkVtCq_r5rs1s_K2bJSNaeUP5To8hfxJTlUZVIiTjmw4fClSvrU_dtuhGaD4Emr9q4pButxpYSpfs9AX3FgxNhUo35X-NfkDwma7nctx3ToLxDUaQpOzRlHVwVOFYCRIFnPdlTNvlu_2UrVjb7qQFji90llpIwlnoimq6Qpt4KMVwEqH-iiPU498-tIwuxdPVq7cYJe2_MM8_eFkF3sVoC5qbsdAqIvup1ZAlIOyepioatTIWbIhr0CBquU3GP0pENnlRwNG8e1Kjej3VJAr5nS9p-bP3GWBH5Y7NbepV-jkomXiJdUVdX5CAqgU2ll8qbNxclu3Wpxnz7IYoan5IFIr9zJPuPVsubye_Ajcp_zdWoFRtc-pUlQszHmZ4keyBC_Cih9ucoJ3tU1dVE6ZH16ynDFsz7Voffgy9d27cyIP",
  "dp": "2MImN914_5mThtC5MM0X7nN2agZybdrwTGdU5EC-VgFAYoB3IpJzd2Lh67fHUqbLpkuq6HhQKRqXDVxw0y3GW0iBWWluqX-ATSKIac2gjFQhogNCklI1cave5-pmZOPR51I-HBaYo0WvywnkJNgUruSB_eM7M5lhbi7UeYpDHaaMW1LCMG6UF2zRC-7HCPXegnlQGwLL_JvOJS9a7H6h2tsw7n7zOeDRnzoi5hyEMTV_AwvA_XKzPSOvDh3_It6H",
  "dq": "iQsvC3JaQfjWVKGkuEaAZfO3jCad-ruTr8yod9zMuTeywQAr4iYylbUxJbnwB73dBzm5K5ue3rFwxjA8UmOWuTeu7P0J3D393SG1aNVuv-QzrX4LhURXVUyxPJv-r8zHV-LH18yWmAMR4qycLnXkM_p76MOoe92gB_987dCc31RSsSnvz4ECjEsRUDjlS7w_gVOHBPj6G1Ew8UkGXwEXW43maJbWbuX8q8X-wm8uadaVaoYd8SoAL2zT-UOtqAQ7",
  "e": "AQAB",
  "key_ops": [
    "sign"
  ],
  "kty": "RSA",
  "n": "8dySRYlmp3Ysv5oZL8WzWRgeVEm5uzW2LSoHJrtLoyG5v0cmjR4NnU_Y8ixtFzlb2pOaGlmEXEK_8Uuh0AkbuKohZuUCZzeRoibS4hFTuoaKNKUey5teJXWd_sl6OeCO2myVq7KXw5DnBxR1P3FVUM-aIYjfX3LCbyh-HyenR0mXD_4DloXKPoQ2Cm_yoOspNcBFkOFniMgXDhuaOybJn6z0GfVLlK4uLxZnvt_XJ3YWgM2gFTEqzVXEl6MQvsZG8-6hrf1uKLjOWt6zoNp5DtPJ2ZB35VerK1Y9wGBffYXz-18ZSP-xHBWsLhTvMWrTvlycIepVem8rNi832TtjfgP062p-QxIxK8O7tVyj3WYQLazEy9uJoTQQVmSoFVczrHwIPvklMi52NKTV_jLMqA9A8R6K-Z2x7fZMZ4qr9cz3HpqoRkSOGfd2O5wLuGRNyIsFeGn51WLdkKrV_oH1aem1n4tLa1Pf3yBlswyf3NSNH6CiHthjsrIIVSWn2xgJ",
  "p": "-kfqpffy0RT7g5bNRUHEmOoWnxBloJ0Frzyn47NkBzuPa_5wE5mSQTPTIA_2TgdYGHX1cAAu6dpyrbmxvp8Vy0T85Fwi_r4LhazrFAfJTtRc2YI13n30Ge5eCetl-qkoZRXge8inJFvlyhdsPKfO2eXacKVMXhsA12m6lpM6ZPmb44uIa1k6xhG-4joK-hUBrxNPqrbeuY_OI3wZ8Utfh8Ka2Jxsxiomp0kuHnEeoNFZ2iKO3VZDNjRUUIpmTnWr",
  "q": "92NnXtf-2IqMJjfe2AjOC1NHu4-A2_USxf2lkBNTK8jxVL4q5x3E3dWkMpgcRhNljfE8uo5_WZcAV8hXD4T5vK0nirFyWNUFzA58wbUDRizF7gf5ph9MV7gfQscyPVQ8lw4qDvdO9VtrmFWRPp-tCFxYDV3I9CIgvnClJvvaAb45l4uhjMztYvL8yV_fw_1X5G-5bvQKm3pOzzm2if3UqrVcaWJsVdxGVCRDDCYrAaZmaGY4TqLwQA_RrFNz0Q0b",
  "qi": "6mP9JfdgM4Ygf9FNc4T0s4jpUy7OHCZajj58WLp3u4ySEtekBEDSGN4xepJkztj_iKhGTPQWFIypFY5DLFL10JxjWFsPZKx-4t8_UrmzHpZEuvJyeKSmFrQ6V_73opDLKmaEv8QMDmuGuJFk96N3EU1Xe4L0JQIRe0GnBU6nu9ygnMS5cYrVl-PRK_dKPTtAYJfX21ZM3p-wqijpFUkzImAW17i6vq7bmgeekZUyf8VkeBrwA11yqSMquWeEvYld",
  "use": "sig",
  "kid": "bbc17ef9fed185bde46864233c15436c"
}`

var ExampleMyPublicKeyJSON = `{
  "alg": "RS256",
  "e": "AQAB",
  "key_ops": [
    "verify"
  ],
  "kty": "RSA",
  "n": "8dySRYlmp3Ysv5oZL8WzWRgeVEm5uzW2LSoHJrtLoyG5v0cmjR4NnU_Y8ixtFzlb2pOaGlmEXEK_8Uuh0AkbuKohZuUCZzeRoibS4hFTuoaKNKUey5teJXWd_sl6OeCO2myVq7KXw5DnBxR1P3FVUM-aIYjfX3LCbyh-HyenR0mXD_4DloXKPoQ2Cm_yoOspNcBFkOFniMgXDhuaOybJn6z0GfVLlK4uLxZnvt_XJ3YWgM2gFTEqzVXEl6MQvsZG8-6hrf1uKLjOWt6zoNp5DtPJ2ZB35VerK1Y9wGBffYXz-18ZSP-xHBWsLhTvMWrTvlycIepVem8rNi832TtjfgP062p-QxIxK8O7tVyj3WYQLazEy9uJoTQQVmSoFVczrHwIPvklMi52NKTV_jLMqA9A8R6K-Z2x7fZMZ4qr9cz3HpqoRkSOGfd2O5wLuGRNyIsFeGn51WLdkKrV_oH1aem1n4tLa1Pf3yBlswyf3NSNH6CiHthjsrIIVSWn2xgJ",
  "use": "sig",
  "kid": "bbc17ef9fed185bde46864233c15436c"
}`

// Example shows signing and validating tokens. It is compiled by go test but
// not run, because it has no Output comment and needs a reachable JWKS URL.
func Example() {
	myPrivateKey := ExampleMyPrivateKey
	publicKeyJSON := ExampleMyPublicKeyJSON

	token, err := GenerateTokenWithStruct(myPrivateKey, ExampleTok)
	if err != nil {
		panic(err)
	}
	fmt.Println("Your JWT:")
	fmt.Println(token)

	t2, err := ValidateJwtWithPublicKey(token, publicKeyJSON, "my-app", "my-api")
	if err != nil {
		panic(err)
	}
	fmt.Println("Validated Token:")
	fmt.Println(t2)

	t3, err := NewJWKSVerifier(
		context.Background(),
		"https://auth.example.com/.well-known/jwks.json",
		"my-app",
		"my-api",
	)
	if err != nil {
		panic(err)
	}
	fmt.Println("Validated Token:")
	fmt.Println(t3)

	// =======
	t4, err := SignTokenWithJWK(myPrivateKey, ExampleTok)
	if err != nil {
		panic(err)
	}
	fmt.Println("Signed Token:")
	fmt.Println(t4)

	t5, err := ValidateJwtWithPublicKey(t4, publicKeyJSON, "my-app", "my-api")
	if err != nil {
		panic(err)
	}
	fmt.Println("Validated Token:")
	fmt.Println(t5)
}
