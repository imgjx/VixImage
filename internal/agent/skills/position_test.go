package skills

import "testing"

func TestNormAnchor(t *testing.T) {
	cases := map[string]string{
		"tl": "tl", "top-left": "tl", "左上": "tl",
		"tc": "tc", "top-center": "tc", "center-top": "tc", "居中上": "tc",
		"mc": "mc", "center": "mc", "正中": "mc", "middle": "mc",
		"mr": "mr", "right": "mr", "右中": "mr",
		"bl": "bl", "bottom_left": "bl", "左下": "bl",
		"br": "br", "bottom-right": "br", "右下": "br",
	}
	for in, want := range cases {
		if got := normAnchor(in); got != want {
			t.Errorf("normAnchor(%q)=%q 期望 %q", in, got, want)
		}
	}
	for _, in := range []string{"", "xyz", "foo"} {
		if got := normAnchor(in); got != "" {
			t.Errorf("normAnchor(%q) 应为空，得到 %q", in, got)
		}
	}
}

func TestResolvePositionAnchor(t *testing.T) {
	const W, H = 1080, 1920
	// 顶部居中：x=(1080-w)/2, y=inset(短边4%=43), 加偏移。
	r := ResolvePosition(PositionSpec{Pos: "tc", OX: 0, OY: 10}, 0, 0, 600, 200, W, H)
	wantX := (W - 600) / 2
	if r.X != wantX || r.Y != 43+10 {
		t.Fatalf("tc 定点: got %+v, want X=%d Y=%d", r, wantX, 43+10)
	}

	// 右下角：x=W-w-inset, y=H-h-inset。
	r = ResolvePosition(PositionSpec{Pos: "br"}, 0, 0, 300, 300, W, H)
	if r.X != W-300-43 || r.Y != H-300-43 {
		t.Fatalf("br 定点: got %+v", r)
	}

	// 正中：框居中。
	r = ResolvePosition(PositionSpec{Pos: "mc"}, 0, 0, 400, 400, W, H)
	if r.X != (W-400)/2 || r.Y != (H-400)/2 {
		t.Fatalf("mc 定点: got %+v", r)
	}

	// 自定义 inset。
	r = ResolvePosition(PositionSpec{Pos: "tl", Inset: 80}, 0, 0, 100, 100, W, H)
	if r.X != 80 || r.Y != 80 {
		t.Fatalf("tl + inset: got %+v", r)
	}
}

func TestResolvePositionPercent(t *testing.T) {
	const W, H = 1000, 2000
	// px/py 用 0~100。
	r := ResolvePosition(PositionSpec{PX: 50, PY: 10, HasPX: true, HasPY: true, PW: 80, PH: 20, HasPW: true, HasPH: true}, 0, 0, 0, 0, W, H)
	if r.X != 500 || r.Y != 200 || r.W != 800 || r.H != 400 {
		t.Fatalf("百分比(0~100): got %+v", r)
	}
	// 0~1 比例。
	r = ResolvePosition(PositionSpec{PX: 0.5, PY: 0.25, HasPX: true, HasPY: true}, 0, 0, 100, 100, W, H)
	if r.X != 500 || r.Y != 500 {
		t.Fatalf("百分比(0~1): got %+v", r)
	}
}

func TestResolvePositionFallback(t *testing.T) {
	// 无定点意图：原样返回绝对坐标（向后兼容）。
	r := ResolvePosition(PositionSpec{}, 120, 340, 500, 260, 1080, 1920)
	if r.X != 120 || r.Y != 340 || r.W != 500 || r.H != 260 {
		t.Fatalf("fallback: got %+v", r)
	}
	// 百分比 + 锚点叠加：先算百分比尺寸，锚点用该尺寸定位，再加偏移。
	r = ResolvePosition(PositionSpec{Pos: "bc", PW: 50, HasPW: true, OY: -20}, 0, 0, 0, 300, 1080, 1920)
	if r.W != 540 {
		t.Fatalf("百分比尺寸未生效: got %+v", r)
	}
	if r.X != (1080-540)/2 || r.Y != 1920-300-43-20 {
		t.Fatalf("百分比+锚点叠加: got %+v", r)
	}
}
