package service

import (
	"math"
	"time"

	"github.com/faisalaffan/faisalaffan-design-system/services/tracking-service/model"
)

// KalmanFilter implements a 4-state discrete Kalman filter for position
// smoothing. The state vector is [lat, lng, v_lat, v_lng].
//
// State transition (constant-velocity model):
//
//	F = | 1  0  dt  0 |
//	    | 0  1  0  dt |
//	    | 0  0  1   0 |
//	    | 0  0  0   1 |
//
// Measurement matrix picks position:
//
//	H = | 1  0  0  0 |
//	    | 0  1  0  0 |
type KalmanFilter struct {
	// State
	x [4]float64 // [lat, lng, v_lat, v_lng]

	// Covariance (4x4 upper-triangular for efficiency)
	P [4][4]float64

	// Process noise factor (tuned experimentally)
	Q float64

	// Last update time
	lastTime time.Time

	// Whether the filter has been initialised
	initialised bool
}

// NewKalmanFilter creates a filter with default process noise.
// q controls how much we trust the model vs measurements (higher = more trust in measurements).
func NewKalmanFilter(q float64) *KalmanFilter {
	if q == 0 {
		q = 1.0
	}
	kf := &KalmanFilter{Q: q}
	// Initial covariance: moderate uncertainty on position, higher on velocity
	kf.P = [4][4]float64{
		{10, 0, 0, 0},
		{0, 10, 0, 0},
		{0, 0, 100, 0},
		{0, 0, 0, 100},
	}
	return kf
}

// Initialise seeds the filter with the first measurement.
func (kf *KalmanFilter) Initialise(lat, lng float64, t time.Time) {
	kf.x = [4]float64{lat, lng, 0, 0}
	kf.lastTime = t
	kf.initialised = true
}

// Reset clears filter state for a new order/driver session.
func (kf *KalmanFilter) Reset() {
	kf.initialised = false
	kf.x = [4]float64{}
	kf.P = [4][4]float64{
		{10, 0, 0, 0},
		{0, 10, 0, 0},
		{0, 0, 100, 0},
		{0, 0, 0, 100},
	}
}

// IsInitialised returns true if the filter has been seeded.
func (kf *KalmanFilter) IsInitialised() bool {
	return kf.initialised
}

// Update runs one predict-update cycle with a GPS measurement.
// accuracy is the reported GPS accuracy in meters — used to adapt R.
// Returns the smoothed position.
func (kf *KalmanFilter) Update(lat, lng float64, accuracy float64, t time.Time) model.SmoothedPosition {
	if !kf.initialised {
		kf.Initialise(lat, lng, t)
	}

	dt := t.Sub(kf.lastTime).Seconds()
	if dt <= 0 {
		dt = 0.1 // minimum dt to avoid division issues
	}
	kf.lastTime = t

	// ---- Predict ----
	kf.predict(dt)

	// ---- Update ----
	// Adaptive measurement noise: scale R by accuracy / 10.0
	rScale := accuracy / 10.0
	if rScale < 1.0 {
		rScale = 1.0
	}
	R := rScale * 5.0 // base R = 5m^2

	// Innovation (residual)
	y0 := lat - kf.x[0]
	y1 := lng - kf.x[1]

	// Innovation covariance: S = H * P * H^T + R
	// Since H = [1 0 0 0; 0 1 0 0], S = P[:2][:2] + R*I
	S00 := kf.P[0][0] + R
	S01 := kf.P[0][1]
	S11 := kf.P[1][1] + R

	// Determinant for 2x2 inversion
	det := S00*S11 - S01*S01
	if math.Abs(det) < 1e-12 {
		det = 1e-12
	}
	invS00 := S11 / det
	invS01 := -S01 / det
	invS11 := S00 / det

	// Kalman Gain: K = P * H^T * S^-1
	K00 := kf.P[0][0]*invS00 + kf.P[0][1]*invS01
	K01 := kf.P[0][0]*invS01 + kf.P[0][1]*invS11
	K10 := kf.P[1][0]*invS00 + kf.P[1][1]*invS01
	K11 := kf.P[1][0]*invS01 + kf.P[1][1]*invS11
	K20 := kf.P[2][0]*invS00 + kf.P[2][1]*invS01
	K21 := kf.P[2][0]*invS01 + kf.P[2][1]*invS11
	K30 := kf.P[3][0]*invS00 + kf.P[3][1]*invS01
	K31 := kf.P[3][0]*invS01 + kf.P[3][1]*invS11

	// State update: x = x + K * y
	kf.x[0] += K00*y0 + K01*y1
	kf.x[1] += K10*y0 + K11*y1
	kf.x[2] += K20*y0 + K21*y1
	kf.x[3] += K30*y0 + K31*y1

	// Covariance update: P = (I - K*H) * P
	kf.covarianceUpdate(K00, K01, K10, K11, K20, K21, K30, K31)

	speed := math.Hypot(kf.x[2], kf.x[3])
	bearing := math.Atan2(kf.x[3], kf.x[2]) * 180.0 / math.Pi
	if bearing < 0 {
		bearing += 360
	}

	return model.SmoothedPosition{
		Lat:     kf.x[0],
		Lng:     kf.x[1],
		Speed:   speed,
		Bearing: bearing,
	}
}

// predict runs the predict step for dt seconds.
func (kf *KalmanFilter) predict(dt float64) {
	// State prediction: x = F * x
	kf.x[0] += kf.x[2] * dt
	kf.x[1] += kf.x[3] * dt
	// velocity unchanged in CV model

	// Covariance prediction: P = F * P * F^T + Q
	// Build F explicitly for clarity
	// F = | 1 0 dt 0 |
	//     | 0 1 0 dt |
	//     | 0 0 1  0 |
	//     | 0 0 0  1 |
	dt2 := dt * dt

	// P_new[0][0] = P[0][0] + 2*dt*P[2][0] + dt2*P[2][2]
	// P_new[0][1] = P[0][1] + dt*P[2][1] + dt*P[3][0] + dt2*P[2][3]
	// etc.
	p00 := kf.P[0][0] + 2*dt*kf.P[0][2] + dt2*kf.P[2][2]
	p01 := kf.P[0][1] + dt*kf.P[0][3] + dt*kf.P[1][2] + dt2*kf.P[2][3]
	p02 := kf.P[0][2] + dt*kf.P[2][2]
	p03 := kf.P[0][3] + dt*kf.P[2][3]
	p11 := kf.P[1][1] + 2*dt*kf.P[1][3] + dt2*kf.P[3][3]
	p12 := kf.P[1][2] + dt*kf.P[2][3]
	p13 := kf.P[1][3] + dt*kf.P[3][3]
	p22 := kf.P[2][2]
	p23 := kf.P[2][3]
	p33 := kf.P[3][3]

	// Add process noise (discrete-time white noise acceleration model)
	// Q_11 = q * dt^3/3, Q_12 = q * dt^2/2, Q_22 = q * dt
	q := kf.Q
	qD := q * dt
	qD2 := q * dt2 / 2.0
	qD3 := q * dt2 * dt / 3.0

	kf.P[0][0] = p00 + qD3 // lat-lat
	kf.P[0][1] = p01
	kf.P[1][0] = p01
	kf.P[1][1] = p11 + qD3 // lng-lng
	kf.P[0][2] = p02 + qD2 // lat-vlat
	kf.P[2][0] = p02 + qD2
	kf.P[0][3] = p03
	kf.P[3][0] = p03
	kf.P[1][2] = p12
	kf.P[2][1] = p12
	kf.P[1][3] = p13 + qD2 // lng-vlng
	kf.P[3][1] = p13 + qD2
	kf.P[2][2] = p22 + qD // vlat-vlat
	kf.P[2][3] = p23
	kf.P[3][2] = p23
	kf.P[3][3] = p33 + qD // vlng-vlng
}

// covarianceUpdate computes P = (I - K*H) * P where H pulls position.
func (kf *KalmanFilter) covarianceUpdate(K00, K01, K10, K11, K20, K21, K30, K31 float64) {
	// I - K*H for position:
	// KH = | K00  K01  0  0 |
	//      | K10  K11  0  0 |
	//      | K20  K21  0  0 |
	//      | K30  K31  0  0 |
	p00 := kf.P[0][0] - K00*kf.P[0][0] - K01*kf.P[1][0]
	p01 := kf.P[0][1] - K00*kf.P[0][1] - K01*kf.P[1][1]
	p02 := kf.P[0][2] - K00*kf.P[0][2] - K01*kf.P[1][2]
	p03 := kf.P[0][3] - K00*kf.P[0][3] - K01*kf.P[1][3]

	p10 := kf.P[1][0] - K10*kf.P[0][0] - K11*kf.P[1][0]
	p11 := kf.P[1][1] - K10*kf.P[0][1] - K11*kf.P[1][1]
	p12 := kf.P[1][2] - K10*kf.P[0][2] - K11*kf.P[1][2]
	p13 := kf.P[1][3] - K10*kf.P[0][3] - K11*kf.P[1][3]

	p20 := kf.P[2][0] - K20*kf.P[0][0] - K21*kf.P[1][0]
	p21 := kf.P[2][1] - K20*kf.P[0][1] - K21*kf.P[1][1]
	p22 := kf.P[2][2] - K20*kf.P[0][2] - K21*kf.P[1][2]
	p23 := kf.P[2][3] - K20*kf.P[0][3] - K21*kf.P[1][3]

	p30 := kf.P[3][0] - K30*kf.P[0][0] - K31*kf.P[1][0]
	p31 := kf.P[3][1] - K30*kf.P[0][1] - K31*kf.P[1][1]
	p32 := kf.P[3][2] - K30*kf.P[0][2] - K31*kf.P[1][2]
	p33 := kf.P[3][3] - K30*kf.P[0][3] - K31*kf.P[1][3]

	kf.P[0][0], kf.P[0][1], kf.P[0][2], kf.P[0][3] = p00, p01, p02, p03
	kf.P[1][0], kf.P[1][1], kf.P[1][2], kf.P[1][3] = p10, p11, p12, p13
	kf.P[2][0], kf.P[2][1], kf.P[2][2], kf.P[2][3] = p20, p21, p22, p23
	kf.P[3][0], kf.P[3][1], kf.P[3][2], kf.P[3][3] = p30, p31, p32, p33
}

// PredictNext returns the expected position at time t without updating the
// filter state. Useful for interpolation between measurements.
func (kf *KalmanFilter) PredictNext(t time.Time) model.SmoothedPosition {
	if !kf.initialised {
		return model.SmoothedPosition{}
	}
	dt := t.Sub(kf.lastTime).Seconds()
	if dt < 0 {
		dt = 0
	}
	return model.SmoothedPosition{
		Lat:     kf.x[0] + kf.x[2]*dt,
		Lng:     kf.x[1] + kf.x[3]*dt,
		Speed:   math.Hypot(kf.x[2], kf.x[3]),
		Bearing: math.Atan2(kf.x[3], kf.x[2]) * 180.0 / math.Pi,
	}
}

// CurrentPosition returns the most recent (updated) position without modification.
func (kf *KalmanFilter) CurrentPosition() model.SmoothedPosition {
	return model.SmoothedPosition{
		Lat:     kf.x[0],
		Lng:     kf.x[1],
		Speed:   math.Hypot(kf.x[2], kf.x[3]),
		Bearing: math.Atan2(kf.x[3], kf.x[2]) * 180.0 / math.Pi,
	}
}
