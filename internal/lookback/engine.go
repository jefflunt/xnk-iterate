package lookback

import (
	"fmt"
	"math"
	"os"
	"path/filepath"

	"github.com/jefflunt/xnk-iterate/internal/interaction"
	"github.com/jefflunt/xnk-iterate/internal/model"
	"github.com/jefflunt/xnk-iterate/internal/ringbuffer"
)

// Result holds the computed lookback window and optional anchor interaction.
type Result struct {
	Range        model.LookbackRange
	AnchorAction *model.InteractionEvent
	CarouselTime []float64
}

// Engine correlates finalized speech boundaries with preceding OS interactions
// and extracts Trigger, Reaction, and optional Carousel frames from the ring buffer.
type Engine struct {
	ActionSearchWindowSeconds float64
	ActionPrePaddingSeconds   float64
	DefaultMinBufferSeconds   float64
	ExtendedThresholdSeconds  float64
}

// New creates a Dynamic Lookback Engine configured with the specification defaults.
func New() *Engine {
	return &Engine{
		ActionSearchWindowSeconds: 10.0,
		ActionPrePaddingSeconds:   1.0,
		DefaultMinBufferSeconds:   3.5,
		ExtendedThresholdSeconds:  15.0,
	}
}

// Calculate determines the dynamic lookback window for an utterance spanning [speechStart, speechEnd].
func (e *Engine) Calculate(speechStart, speechEnd float64, timeline *interaction.Timeline) Result {
	if speechEnd < speechStart {
		speechEnd = speechStart
	}
	utteranceDuration := speechEnd - speechStart

	var anchor *model.InteractionEvent
	if timeline != nil {
		searchStart := math.Max(0, speechStart-e.ActionSearchWindowSeconds)
		candidates := timeline.InRange(searchStart, speechStart)
		if len(candidates) > 0 {
			last := candidates[len(candidates)-1]
			anchor = &last
		}
	}

	var windowStart float64
	var triggerTS float64
	if anchor != nil {
		windowStart = math.Max(0, anchor.Timestamp-e.ActionPrePaddingSeconds)
		triggerTS = anchor.Timestamp
	} else {
		buffer := math.Max(e.DefaultMinBufferSeconds, utteranceDuration*0.25)
		windowStart = math.Max(0, speechStart-buffer)
		triggerTS = math.Max(0, speechStart-e.DefaultMinBufferSeconds)
	}

	rng := model.LookbackRange{
		StartSeconds:   windowStart,
		EndSeconds:     speechEnd,
		TriggerSeconds: triggerTS,
		Duration:       math.Max(0, speechEnd-windowStart),
	}

	var carousel []float64
	if utteranceDuration > e.ExtendedThresholdSeconds {
		// Extract 5 evenly distributed frames across [windowStart, speechEnd].
		const count = 5
		carousel = make([]float64, 0, count)
		step := rng.Duration / float64(count-1)
		for i := 0; i < count; i++ {
			carousel = append(carousel, windowStart+float64(i)*step)
		}
	}

	return Result{
		Range:        rng,
		AnchorAction: anchor,
		CarouselTime: carousel,
	}
}

// ExtractFrames writes Trigger, Reaction, and optional Carousel frames from rb into framesDir.
func (e *Engine) ExtractFrames(
	utteranceID int,
	res Result,
	rb *ringbuffer.FrameRingBuffer,
	framesDir string,
) (triggerPath string, reactionPath string, carouselPaths []string, snapshots []model.FrameSnapshot, err error) {
	if rb == nil || rb.Len() == 0 {
		return "", "", nil, nil, nil
	}
	if err := os.MkdirAll(framesDir, 0o755); err != nil {
		return "", "", nil, nil, err
	}

	saveFrame := func(targetTS float64, role model.FrameRole, suffix string) (string, *model.FrameSnapshot, error) {
		f, ok := rb.Nearest(targetTS)
		if !ok || len(f.Data) == 0 {
			return "", nil, nil
		}
		ext := f.Format
		if ext == "" {
			ext = "png"
		}
		name := fmt.Sprintf("frame_%02d_%s_%.2fs.%s", utteranceID, suffix, f.Timestamp, ext)
		absPath := filepath.Join(framesDir, name)
		if err := os.WriteFile(absPath, f.Data, 0o644); err != nil {
			return "", nil, err
		}
		snap := &model.FrameSnapshot{
			Timestamp:    f.Timestamp,
			RelativePath: filepath.Join("frames", name),
			AbsolutePath: absPath,
			Role:         role,
			UtteranceID:  utteranceID,
		}
		return absPath, snap, nil
	}

	if p, snap, err := saveFrame(res.Range.TriggerSeconds, model.FrameRoleTrigger, "trigger"); err != nil {
		return "", "", nil, nil, err
	} else if snap != nil {
		triggerPath = p
		snapshots = append(snapshots, *snap)
	}

	if p, snap, err := saveFrame(res.Range.EndSeconds, model.FrameRoleReaction, "reaction"); err != nil {
		return "", "", nil, nil, err
	} else if snap != nil {
		reactionPath = p
		snapshots = append(snapshots, *snap)
	}

	for idx, ts := range res.CarouselTime {
		suffix := fmt.Sprintf("carousel_%02d", idx+1)
		if p, snap, err := saveFrame(ts, model.FrameRoleCarousel, suffix); err != nil {
			return "", "", nil, nil, err
		} else if snap != nil {
			carouselPaths = append(carouselPaths, p)
			snapshots = append(snapshots, *snap)
		}
	}

	return triggerPath, reactionPath, carouselPaths, snapshots, nil
}
