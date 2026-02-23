package rules

import "testing"

func BenchmarkParseBlockRules(b *testing.B) {
	const rulesString = `
default {
  remove resp_header X-Secret
  add resp_header X-Custom-Header custom-value
}

header X-Test-Header {
  set header X-Remote-Type public
  remote 127.0.0.1 | remote 192.168.0.0/16 {
    set header X-Remote-Type private
  }
}

path glob(/api/admin/*) {
	cookie session-id {
		set header X-Session-ID $cookie(session-id)
	}
}

!remote 192.168.0.0/16 {
  !header X-User-Role admin & !header X-User-Role user {
    error 403 "Access denied"
  } elif remote 127.0.0.1 {
    header X-User-Role staff {
      set header X-User-Role staff
    }
  } else {
    error 403 "Access denied"
  }
}
`

	var rules Rules
	err := rules.Parse(rulesString)
	if err != nil {
		b.Fatal(err)
	}

	for b.Loop() {
		var rules Rules
		_ = rules.Parse(rulesString)
	}
}
