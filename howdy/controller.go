package howdy

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	appName         = "multi-john"
	workerComponent = "worker"
	inputVolumeName = "input"
)

type Controller struct {
	client *kubernetes.Clientset
	config ControllerConfig
	log    *zap.SugaredLogger
}

type ControllerConfig struct {
	Namespace               string
	Image                   string
	ImagePullPolicy         string
	EtcdEndpoint            string
	JohnPath                string
	InputPath               string
	InputFile               string
	LogLevel                string
	DefaultJohnFlags        string
	DefaultTotalNodes       int32
	TTLSecondsAfterFinished int32
	ActiveDeadlineSeconds   int64
	RequestCPU              string
	RequestMemory           string
	LimitCPU                string
	LimitMemory             string
}

type CreateJobRequest struct {
	Name                    string            `json:"name"`
	Hashes                  string            `json:"hashes"`
	JohnFlags               string            `json:"johnFlags"`
	TotalNodes              int32             `json:"totalNodes"`
	Parallelism             int32             `json:"parallelism"`
	TTLSecondsAfterFinished int32             `json:"ttlSecondsAfterFinished"`
	ActiveDeadlineSeconds   int64             `json:"activeDeadlineSeconds"`
	NodeSelector            map[string]string `json:"nodeSelector"`
}

type CreatedJob struct {
	RunID      string `json:"runID"`
	JobName    string `json:"jobName"`
	SecretName string `json:"secretName"`
}

type JobSummary struct {
	Name        string     `json:"name"`
	RunID       string     `json:"runID"`
	Active      int32      `json:"active"`
	Succeeded   int32      `json:"succeeded"`
	Failed      int32      `json:"failed"`
	Completions int32      `json:"completions"`
	Parallelism int32      `json:"parallelism"`
	CreatedAt   *time.Time `json:"createdAt,omitempty"`
}

func NewControllerFromEnv(logger *zap.Logger) (*Controller, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &Controller{
		client: client,
		config: controllerConfigFromEnv(),
		log:    logger.Sugar(),
	}, nil
}

func controllerConfigFromEnv() ControllerConfig {
	return ControllerConfig{
		Namespace:               envString("MULTI_JOHN_NAMESPACE", namespace()),
		Image:                   envString("MULTI_JOHN_IMAGE", "multi-john:latest"),
		ImagePullPolicy:         envString("MULTI_JOHN_IMAGE_PULL_POLICY", string(corev1.PullIfNotPresent)),
		EtcdEndpoint:            envString("ETCD_ADVERTISE_CLIENT_URLS", "etcd:2379"),
		JohnPath:                envString("MULTI_JOHN_JOHN_PATH", "/jtr/run/john"),
		InputPath:               envString("MULTI_JOHN_INPUT_PATH", "/input"),
		InputFile:               envString("MULTI_JOHN_INPUT_FILE", "hashes"),
		LogLevel:                envString("MULTI_JOHN_LOG_LEVEL", "info"),
		DefaultJohnFlags:        envString("MULTI_JOHN_DEFAULT_JOHN_FLAGS", ""),
		DefaultTotalNodes:       envInt32("MULTI_JOHN_DEFAULT_TOTAL_NODES", 2),
		TTLSecondsAfterFinished: envInt32("MULTI_JOHN_JOB_TTL_SECONDS_AFTER_FINISHED", 86400),
		ActiveDeadlineSeconds:   envInt64("MULTI_JOHN_JOB_ACTIVE_DEADLINE_SECONDS", 86400),
		RequestCPU:              envString("MULTI_JOHN_WORKER_REQUEST_CPU", "250m"),
		RequestMemory:           envString("MULTI_JOHN_WORKER_REQUEST_MEMORY", "64Mi"),
		LimitCPU:                envString("MULTI_JOHN_WORKER_LIMIT_CPU", "500m"),
		LimitMemory:             envString("MULTI_JOHN_WORKER_LIMIT_MEMORY", "128Mi"),
	}
}

func (c *Controller) CreateJob(ctx context.Context, req CreateJobRequest) (CreatedJob, error) {
	if strings.TrimSpace(req.Hashes) == "" {
		return CreatedJob{}, fmt.Errorf("hashes are required")
	}
	if len(req.Hashes) > 4*1024*1024 {
		return CreatedJob{}, fmt.Errorf("hash input is too large")
	}
	if strings.TrimSpace(req.JohnFlags) == "" {
		req.JohnFlags = c.config.DefaultJohnFlags
	}

	totalNodes := req.TotalNodes
	if totalNodes < 1 {
		totalNodes = c.config.DefaultTotalNodes
	}
	parallelism := req.Parallelism
	if parallelism < 1 {
		parallelism = totalNodes
	}
	if parallelism > totalNodes {
		return CreatedJob{}, fmt.Errorf("parallelism cannot exceed totalNodes")
	}

	ttl := req.TTLSecondsAfterFinished
	if ttl < 1 {
		ttl = c.config.TTLSecondsAfterFinished
	}
	deadline := req.ActiveDeadlineSeconds
	if deadline < 1 {
		deadline = c.config.ActiveDeadlineSeconds
	}

	runID := runID(req.Name)
	jobName := runID
	secretName := runID + "-in"
	labels := map[string]string{
		"app.kubernetes.io/name":       appName,
		"app.kubernetes.io/component":  workerComponent,
		"app.kubernetes.io/managed-by": "multi-john",
		"multi-john/run-id":            runID,
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: c.config.Namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			c.config.InputFile: []byte(req.Hashes),
		},
	}
	createdSecret, err := c.client.CoreV1().Secrets(c.config.Namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return CreatedJob{}, err
	}

	job, err := c.client.BatchV1().Jobs(c.config.Namespace).Create(ctx, c.jobSpec(req, jobName, secretName, runID, totalNodes, parallelism, ttl, deadline, labels), metav1.CreateOptions{})
	if err != nil {
		if deleteErr := c.client.CoreV1().Secrets(c.config.Namespace).Delete(ctx, secretName, metav1.DeleteOptions{}); deleteErr != nil && !apierrors.IsNotFound(deleteErr) {
			c.log.Error(deleteErr)
		}
		return CreatedJob{}, err
	}

	createdSecret.OwnerReferences = []metav1.OwnerReference{
		*metav1.NewControllerRef(job, batchv1.SchemeGroupVersion.WithKind("Job")),
	}
	if _, err := c.client.CoreV1().Secrets(c.config.Namespace).Update(ctx, createdSecret, metav1.UpdateOptions{}); err != nil {
		c.log.Warnf("created job %s but could not attach owner reference to secret %s: %v", jobName, secretName, err)
	}

	return CreatedJob{RunID: runID, JobName: jobName, SecretName: secretName}, nil
}

func (c *Controller) jobSpec(req CreateJobRequest, jobName, secretName, runID string, totalNodes, parallelism, ttl int32, deadline int64, labels map[string]string) *batchv1.Job {
	mode := batchv1.IndexedCompletion
	backoffLimit := int32(0)
	inputFile := c.config.InputPath + "/" + c.config.InputFile

	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: c.config.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			Completions:             &totalNodes,
			Parallelism:             &parallelism,
			CompletionMode:          &mode,
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			ActiveDeadlineSeconds:   &deadline,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					NodeSelector:  req.NodeSelector,
					Containers: []corev1.Container{
						{
							Name:            "worker",
							Image:           c.config.Image,
							ImagePullPolicy: corev1.PullPolicy(c.config.ImagePullPolicy),
							Command:         []string{"./multijohn"},
							Args: []string{
								"--mode=worker",
								"--johnFile=" + inputFile,
								"--johnFlags=" + req.JohnFlags,
								"--logLevel=" + c.config.LogLevel,
							},
							Env: []corev1.EnvVar{
								{Name: "ETCD_ADVERTISE_CLIENT_URLS", Value: c.config.EtcdEndpoint},
								{Name: "JOHN_PATH", Value: c.config.JohnPath},
								{Name: "TOTAL_NODES", Value: strconv.Itoa(int(totalNodes))},
								{Name: "MULTI_JOHN_RUN_ID", Value: runID},
								{
									Name: "MULTI_JOHN_NODE_INDEX",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.annotations['batch.kubernetes.io/job-completion-index']",
										},
									},
								},
							},
							Resources:    c.resources(),
							VolumeMounts: []corev1.VolumeMount{{Name: inputVolumeName, MountPath: c.config.InputPath, ReadOnly: true}},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: inputVolumeName,
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{SecretName: secretName},
							},
						},
					},
				},
			},
		},
	}
}

func (c *Controller) resources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(c.config.RequestCPU),
			corev1.ResourceMemory: resource.MustParse(c.config.RequestMemory),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(c.config.LimitCPU),
			corev1.ResourceMemory: resource.MustParse(c.config.LimitMemory),
		},
	}
}

func (c *Controller) ListJobs(ctx context.Context) ([]JobSummary, error) {
	re, err := c.client.BatchV1().Jobs(c.config.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=multi-john,app.kubernetes.io/component=worker",
	})
	if err != nil {
		return nil, err
	}
	jobs := make([]JobSummary, 0, len(re.Items))
	for _, job := range re.Items {
		createdAt := job.CreationTimestamp.Time
		summary := JobSummary{
			Name:      job.Name,
			RunID:     job.Labels["multi-john/run-id"],
			Active:    job.Status.Active,
			Succeeded: job.Status.Succeeded,
			Failed:    job.Status.Failed,
			CreatedAt: &createdAt,
		}
		if job.Spec.Completions != nil {
			summary.Completions = *job.Spec.Completions
		}
		if job.Spec.Parallelism != nil {
			summary.Parallelism = *job.Spec.Parallelism
		}
		jobs = append(jobs, summary)
	}
	return jobs, nil
}

var invalidDNSLabel = regexp.MustCompile(`[^a-z0-9-]+`)

func runID(name string) string {
	slug := strings.ToLower(strings.TrimSpace(name))
	slug = invalidDNSLabel.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "run"
	}
	if len(slug) > 40 {
		slug = slug[:40]
		slug = strings.Trim(slug, "-")
	}
	return "multi-john-" + slug + "-" + strings.Split(uuid.NewString(), "-")[0]
}

func namespace() string {
	if b, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
		return strings.TrimSpace(string(b))
	}
	return "default"
}

func envString(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func envInt32(key string, fallback int32) int32 {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 32); err == nil && parsed > 0 {
			return int32(parsed)
		}
	}
	return fallback
}

func envInt64(key string, fallback int64) int64 {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}
