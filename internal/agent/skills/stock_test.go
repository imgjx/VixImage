package skills

import "testing"

func TestParseOpenverse(t *testing.T) {
	raw := []byte(`{"results":[{"url":"http://a/1.jpg","thumbnail":"http://a/t.jpg","title":"cat","creator":"alice","license":"cc0","source":"flickr"},{"url":"","title":"no url"}]}`)
	imgs, err := parseOpenverse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 2 {
		t.Fatalf("got %d results", len(imgs))
	}
	if imgs[0].URL != "http://a/1.jpg" || imgs[0].Author != "alice" || imgs[0].Source != "flickr" {
		t.Fatalf("bad first result: %+v", imgs[0])
	}
}

func TestParsePexels(t *testing.T) {
	raw := []byte(`{"photos":[{"id":42,"photographer":"bob","src":{"original":"http://p/o.jpg","medium":"http://p/m.jpg"}}]}`)
	imgs, err := parsePexels(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 || imgs[0].URL != "http://p/o.jpg" || imgs[0].Author != "bob" {
		t.Fatalf("bad result: %+v", imgs)
	}
}

func TestParsePixabay(t *testing.T) {
	raw := []byte(`{"hits":[{"webformatURL":"http://pix/w.jpg","largeImageURL":"http://pix/l.jpg","previewURL":"http://pix/p.jpg","user":"carol","tags":"mountain,sky"}]}`)
	imgs, err := parsePixabay(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(imgs) != 1 || imgs[0].URL != "http://pix/l.jpg" || imgs[0].Author != "carol" {
		t.Fatalf("bad result: %+v", imgs)
	}
}

func TestSearchStockValidation(t *testing.T) {
	if _, err := SearchStock("openverse", "", 1, StockConfig{}); err == nil {
		t.Fatal("empty query should fail")
	}
	if _, err := SearchStock("unknown", "cat", 1, StockConfig{}); err == nil {
		t.Fatal("unknown site should fail")
	}
	if _, err := SearchStock("pexels", "cat", 1, StockConfig{}); err == nil {
		t.Fatal("pexels without key should fail")
	}
}
