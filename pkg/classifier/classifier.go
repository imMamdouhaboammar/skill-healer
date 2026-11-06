package classifier

import (
	"strings"
	"unicode"

	"github.com/imMamdouhaboammar/skill-healer/pkg/types"
)

// Result explains which domain rule matched and why.
type Result struct {
	Domain     types.Domain
	Confidence float64
	Basis      string
	Evidence   []string
}

var domainPriority = []types.Domain{
	types.DomainSecurity, types.DomainReverseEng, types.DomainTesting,
	types.DomainDevOps, types.DomainAI, types.DomainContent,
	types.DomainFrontend, types.DomainBackend, types.DomainOrchestration,
}

var domainKeywords = map[types.Domain][]string{
	types.DomainFrontend: {
		"react", "vue", "angular", "tailwind", "framer", "gsap", "anime", "css", "html",
		"design", "taste", "ui", "ux", "claymorphism", "glassmorphism", "neobrutalism",
		"shadcn", "remotion", "three-js", "canvas", "svg", "typography", "bento", "retro",
		"vintage", "matrix", "paper", "neon", "clean", "bold", "creative", "contemporary",
		"flat", "dithered", "doodle", "friendly", "gradient", "minimal", "mono", "pacman",
		"power", "premium", "refined", "riso", "roku", "sleek", "spacious", "square",
		"stitch", "tetris", "vibrant", "artistic", "basic", "cafe", "colorful", "corporate",
		"cosmic", "editorial", "fantasy", "futuristic", "material", "modern", "pneumorphism",
		"21st", "style", "carousel", "animation", "figma",
	},
	types.DomainBackend: {
		"api", "fastapi", "django", "flask", "spring", "aspnet", "dotnet", "laravel", "express",
		"hono", "koa", "graphql", "trpc", "sql", "postgres", "mysql", "prisma", "drizzle",
		"typeorm", "kysely", "supabase", "redis", "rabbitmq", "mqtt", "kafka", "mongodb",
		"grpc", "rest", "orm", "database", "c-sharp", "java", "go", "php", "rust", "elixir",
		"ruby", "python", "node", "deno", "bun", "backend",
	},
	types.DomainDevOps: {
		"docker", "kubernetes", "k8s", "terraform", "azure", "gcp", "google-cloud", "aws",
		"cloudflare", "helm", "istio", "service-mesh", "vercel", "netlify", "railway", "devops",
		"ci-cd", "github-actions", "gitlab", "homelab", "serverless", "workers", "monitoring",
		"prometheus", "grafana", "datadog", "infra", "deploy",
	},
	types.DomainOrchestration: {
		"agent", "kernel", "gsd", "fable", "sya", "ralph", "superpowers", "plan", "task",
		"orchestrat", "harness", "loop", "handoff", "delegate", "subagent", "team", "council",
		"reflex", "mythos", "context", "session", "prompt", "workstream", "workflow",
	},
	types.DomainTesting: {
		"test", "guard", "no-mistakes", "jest", "vitest", "playwright", "cypress", "pytest",
		"rspec", "tdd", "benchmark", "eval", "verify", "verification", "audit-uat", "smoke",
		"coverage", "selenium", "puppeteer", "qa", "check",
	},
	types.DomainSecurity: {
		"secur", "safety", "auth", "jwt", "gitleaks", "secret", "hipaa", "pci", "gdpr",
		"stride", "threat", "bounty", "sast", "penetration", "firewall", "hardening",
		"permission", "turnstile", "sandbox", "vulnerability", "credential", "access control",
	},
	types.DomainAI: {
		"deep-learning", "machine-learning", "pytorch", "jax", "transformers", "pandas",
		"numpy", "scipy", "scikit", "jupyter", "data-analyst", "data-science", "rag", "llm",
		"embedding", "vector", "fine-tuning", "biomolecular", "cheminformatics", "alphafold",
		"genome", "protein", "ncbi", "dbsnp", "clinvar", "chembl", "interpro", "pdb",
	},
	types.DomainReverseEng: {
		"rea", "xerj", "repowise", "debug", "reverse-engineer", "binary", "protocol", "frida",
		"ghidra", "rizin", "gdb", "packet", "wireshark", "ast-grep", "compiler", "profil",
		"disassembler", "trace", "decompile",
	},
	types.DomainContent: {
		"content", "marketing", "copywriting", "seo", "blog", "video", "elevenlabs", "podcast",
		"avatar", "audio", "tts", "stt", "social", "twitter", "linkedin", "image", "banner",
		"pitch-deck", "slides", "sales", "lead", "email", "press-release", "article",
	},
}

func words(value string) string {
	parts := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return " " + strings.Join(parts, " ") + " "
}

func containsTerm(text, term string) bool {
	if strings.Contains(text, " "+strings.TrimSpace(words(term))+" ") {
		return true
	}
	for _, stem := range []string{"secur", "orchestrat", "profil"} {
		if term != stem {
			continue
		}
		for _, word := range strings.Fields(text) {
			if strings.HasPrefix(word, stem) {
				return true
			}
		}
	}
	return false
}

// ClassifyDetailed provides deterministic weighted matching over words and descriptions.
// Confidence describes rule strength, not a calibrated statistical probability.
func ClassifyDetailed(name, description string) Result {
	normalizedName, normalizedDescription := words(name), words(description)
	best := Result{Domain: types.DomainGeneral, Basis: "unclassified"}
	bestScore := 0
	for _, domain := range domainPriority {
		score := 0
		evidence := make([]string, 0, 3)
		for _, keyword := range domainKeywords[domain] {
			if containsTerm(normalizedName, keyword) {
				score += 2
				evidence = append(evidence, "name:"+keyword)
			}
			if containsTerm(normalizedDescription, keyword) {
				score += 4
				evidence = append(evidence, "description:"+keyword)
			}
		}
		if score > bestScore {
			strength := 0.5 + float64(score)*0.045
			if strength > 0.95 {
				strength = 0.95
			}
			basis := "name"
			if containsEvidencePrefix(evidence, "description:") {
				basis = "description"
			}
			if containsEvidencePrefix(evidence, "description:") && containsEvidencePrefix(evidence, "name:") {
				basis = "name+description"
			}
			best = Result{Domain: domain, Confidence: strength, Basis: basis, Evidence: evidence}
			bestScore = score
		}
	}
	return best
}

func containsEvidencePrefix(evidence []string, prefix string) bool {
	for _, entry := range evidence {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}

func ClassifyName(name string) types.Domain {
	return ClassifyDetailed(name, "").Domain
}
