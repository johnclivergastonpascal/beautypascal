package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
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
	URL             string           `json:"url"`
	Categoria       string           `json:"categoria"`
	Subcategoria    string           `json:"subcategoria"`
	Ubicacion       string           `json:"ubicacion"`
	Titulo          string           `json:"titulo"`
	ImagenesGrandes []string         `json:"imagenes"`
	Colores         []Color          `json:"colores"`
	Tamaños         []string         `json:"tamaños"`
	DescripcionURL  string           `json:"descripcion_url"`
	Precios         []Precio         `json:"precios"`
	BloqueLogistico *BloqueLogistico `json:"bloque_logistico"`
}

// ------------------ Variables globales ------------------

var productos []Producto
var rng = rand.New(rand.NewSource(time.Now().UnixNano()))

// ------------------ Función para randomizar valores ------------------

func randomEntre(min, max float64) float64 {
	return min + rng.Float64()*(max-min)
}

// ------------------ Procesar Bloque Logístico ------------------

func parseBloqueLogistico(bl string) *BloqueLogistico {
	if bl == "" {
		return nil
	}

	result := &BloqueLogistico{}
	partes := strings.Split(bl, "||")

	for _, parte := range partes {
		p := strings.TrimSpace(parte)
		if p == "" {
			continue
		}

		var fee string
		if strings.Contains(p, "Premium") {
			fee = fmt.Sprintf("$%.2f", randomEntre(34.50, 65.05))
		} else if strings.Contains(p, "Standard") {
			fee = fmt.Sprintf("$%.2f", randomEntre(24.90, 54.09))
		} else if strings.Contains(p, "Economy") {
			fee = fmt.Sprintf("$%.2f", randomEntre(8.88, 13.99))
		}

		entrega := ""
		if idx := strings.Index(p, "Guaranteed delivery:"); idx != -1 {
			tmp := p[idx+len("Guaranteed delivery:"):]
			tmp = strings.TrimSpace(strings.Split(tmp, ",")[0])
			entrega = tmp
		}

		porcentaje := ""
		if strings.Contains(p, "% delivered") {
			partes := strings.Split(p, ",")
			for _, s := range partes {
				if strings.Contains(s, "% delivered") {
					porcentaje = strings.TrimSpace(strings.Split(s, "%")[0]) + "%"
					break
				}
			}
		}

		bloque := &Bloque{
			ShippingFee:        fee,
			GuaranteedDelivery: entrega,
			Entregas15Dias:     porcentaje,
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

		// 🔹 Procesar bloque logístico con random
		if val, ok := item["bloque_logistico"].(string); ok {
			p.BloqueLogistico = parseBloqueLogistico(val)
		}

		// 🔹 Calcular precio ajustado (cantidad * valor + 1.80)
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

// ------------------ Utilidades ------------------

func parsePrecio(valor string) float64 {
	valor = strings.ReplaceAll(valor, "$", "")
	valor = strings.ReplaceAll(valor, ",", "")
	p, _ := strconv.ParseFloat(valor, 64)
	return p
}

// ------------------ Endpoints ------------------

func getAllProductos(w http.ResponseWriter, r *http.Request) {
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

	start := (page - 1) * limit
	end := start + limit
	if start >= len(productos) {
		start = len(productos)
	}
	if end > len(productos) {
		end = len(productos)
	}

	response := map[string]interface{}{
		"page":      page,
		"limit":     limit,
		"total":     len(productos),
		"productos": productos[start:end],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ------------------ MAIN ------------------

func main() {
	loadJSON()

	mux := http.NewServeMux()
	mux.HandleFunc("/productos", getAllProductos)

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
