package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rs/cors"
)

// ------------------ Estructuras ------------------

type Precio struct {
	Cantidad string `json:"cantidad"`
	Valor    string `json:"valor"`
}

type Color struct {
	Nombre string `json:"nombre"`
	Imagen string `json:"imagen,omitempty"`
}

type Bloque struct {
	ShippingFee        string `json:"shipping_fee"`
	GuaranteedDelivery string `json:"guaranteed_delivery"`
	Entregas15Dias     string `json:"entregas_15_dias"`
}

type BloqueLogistico struct {
	Premium  *Bloque `json:"premium,omitempty"`
	Standard *Bloque `json:"standard,omitempty"`
	Economy  *Bloque `json:"economy,omitempty"`
}

type Producto struct {
	URL             string            `json:"url"`
	Categoria       string            `json:"categoria"`
	Ubicacion       string            `json:"ubicacion"`
	Titulo          string            `json:"titulo"`
	ImagenesGrandes []string          `json:"imagenes"`
	Colores         []Color           `json:"colores"`
	Tamaños         []string          `json:"tamaños"`
	Precios         []Precio          `json:"precios"`
	BloqueLogistico *BloqueLogistico  `json:"bloque_logistico"`
	Detalles        map[string]string `json:"detalles"` // 👈 cambio aquí
}

// ------------------ Variables globales ------------------

var productos []Producto
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// ------------------ Funciones auxiliares ------------------

func randomEntre(min, max float64) float64 {
	return min + rng.Float64()*(max-min)
}

func parsePrecio(valor string) float64 {
	// Limpieza
	valor = strings.ReplaceAll(valor, "$", "")
	valor = strings.ReplaceAll(valor, ",", "")
	valor = strings.TrimSpace(valor)

	// Detectar si es un rango "3.99-4.72"
	if strings.Contains(valor, "-") {
		partes := strings.Split(valor, "-")
		if len(partes) == 2 {
			// Tomar el último valor del rango (el mayor)
			valor = strings.TrimSpace(partes[1])
		}
	}

	// Convertir a float64
	p, _ := strconv.ParseFloat(valor, 64)
	return p
}

func parseBloqueLogistico(bl string) *BloqueLogistico {
	if bl == "" {
		return nil
	}

	result := &BloqueLogistico{}
	partes := strings.Split(bl, "||")

	rePrice := regexp.MustCompile(`\$\d+(\.\d+)?`) // buscar $xx.xx
	var premiumFee float64

	for _, parte := range partes {
		p := strings.TrimSpace(parte)
		if p == "" {
			continue
		}

		var fee float64

		switch {
		case strings.Contains(p, "Premium"):
			// 🔹 Extraer todos los precios en Premium
			if matches := rePrice.FindAllString(p, -1); len(matches) >= 2 {
				// 🔹 Tomar el segundo valor $33.83
				val := strings.ReplaceAll(matches[1], "$", "")
				val = strings.ReplaceAll(val, ",", "")
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					fee = f
				}
			}
			// 🔹 Sumar 1.20
			fee += 1.20
			premiumFee = fee // 🔹 Guardamos para usarlo en Standard/Economy

		case strings.Contains(p, "Standard"):
			// 🔹 Aleatorio menor que Premium
			max := premiumFee - 0.01
			if max < 0.5 {
				max = 0.5
			}
			fee = randomEntre(0.90, max)

		case strings.Contains(p, "Economy"):
			// 🔹 Aleatorio menor que Premium
			max := premiumFee - 0.01
			if max < 0.5 {
				max = 0.5
			}
			fee = randomEntre(0.50, max)
		}

		feeStr := fmt.Sprintf("$%.2f", fee)

		entrega := ""
		if idx := strings.Index(p, "Guaranteed delivery:"); idx != -1 {
			tmp := p[idx+len("Guaranteed delivery:"):]
			tmp = strings.TrimSpace(strings.Split(tmp, ",")[0])
			entrega = tmp
		}

		bloque := &Bloque{
			ShippingFee:        feeStr,
			GuaranteedDelivery: entrega,
		}

		switch {
		case strings.Contains(p, "Premium"):
			result.Premium = bloque
		case strings.Contains(p, "Standard"):
			result.Standard = bloque
		case strings.Contains(p, "Economy"):
			result.Economy = bloque
		}
	}

	return result
}

// ------------------ Cargar JSON ------------------

func loadJSON() {
	path := "productos.json"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "api/productos.json"
	}

	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("❌ Error abriendo JSON: %v", err)
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("❌ Error leyendo JSON: %v", err)
	}

	var raw []map[string]interface{}
	err = json.Unmarshal(bytes, &raw)
	if err != nil {
		log.Fatalf("❌ Error parseando JSON: %v", err)
	}

	for _, item := range raw {
		p := Producto{}
		data, _ := json.Marshal(item)
		json.Unmarshal(data, &p)

		if val, ok := item["bloque_logistico"].(string); ok {
			p.BloqueLogistico = parseBloqueLogistico(val)
		}

		if len(p.Precios) > 0 {
			pr := &p.Precios[0]
			var cantidad float64
			for _, word := range strings.Fields(pr.Cantidad) {
				clean := strings.Trim(word, " -pieces")
				if n, err := strconv.ParseFloat(clean, 64); err == nil {
					cantidad = n
					break
				}
			}

			valor := parsePrecio(pr.Valor)
			if cantidad > 0 && valor > 0 {
				total := (cantidad * valor) + 1.80
				pr.Valor = fmt.Sprintf("$%.2f", total)
			}
		}

		productos = append(productos, p)
	}

	fmt.Printf("✅ %d productos cargados correctamente desde %s\n", len(productos), path)
}

// ------------------ Endpoints ------------------

// /productos — lista general con filtros y paginación
func getAllProductos(w http.ResponseWriter, r *http.Request) {
	query := strings.ToLower(r.URL.Query().Get("q"))
	categoria := strings.ToLower(r.URL.Query().Get("categoria"))
	ubicacion := strings.ToLower(r.URL.Query().Get("ubicacion"))

	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page, _ := strconv.Atoi(pageStr)
	limit, _ := strconv.Atoi(limitStr)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 20
	}

	var filtrados []Producto

	if categoria == "all" {
		filtrados = make([]Producto, len(productos))
		copy(filtrados, productos)
		rng.Shuffle(len(filtrados), func(i, j int) {
			filtrados[i], filtrados[j] = filtrados[j], filtrados[i]
		})
	} else {
		for _, p := range productos {
			tituloMatch := query == "" || strings.Contains(strings.ToLower(p.Titulo), query)
			categoriaMatch := categoria == "" || strings.Contains(strings.ToLower(p.Categoria), categoria)
			ubicacionMatch := ubicacion == "" || strings.Contains(strings.ToLower(p.Ubicacion), ubicacion)
			if tituloMatch && categoriaMatch && ubicacionMatch {
				filtrados = append(filtrados, p)
			}
		}
	}

	start := (page - 1) * limit
	end := start + limit
	if start >= len(filtrados) {
		start = len(filtrados)
	}
	if end > len(filtrados) {
		end = len(filtrados)
	}

	response := map[string]interface{}{
		"page":      page,
		"limit":     limit,
		"total":     len(filtrados),
		"productos": filtrados[start:end],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// /productos/recomendados — devuelve 10 productos aleatorios
func getProductosRecomendados(w http.ResponseWriter, r *http.Request) {
	const cantidad = 10
	if len(productos) == 0 {
		http.Error(w, "No hay productos cargados", http.StatusInternalServerError)
		return
	}

	// Copiar y mezclar aleatoriamente
	copia := make([]Producto, len(productos))
	copy(copia, productos)
	rng.Shuffle(len(copia), func(i, j int) {
		copia[i], copia[j] = copia[j], copia[i]
	})

	// Tomar los primeros 10
	limite := cantidad
	if len(copia) < cantidad {
		limite = len(copia)
	}
	recomendados := copia[:limite]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"total":     limite,
		"productos": recomendados,
	})
}

// ------------------ MAIN ------------------

func main() {
	loadJSON()

	mux := http.NewServeMux()
	mux.HandleFunc("/productos", getAllProductos)
	mux.HandleFunc("/productos/recomendados", getProductosRecomendados) // ✅ nuevo endpoint

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	handler := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"*"},
		AllowCredentials: true,
	}).Handler(mux)

	fmt.Println("🚀 Servidor corriendo en http://localhost:" + port)
	log.Fatal(http.ListenAndServe(":"+port, handler))
}
