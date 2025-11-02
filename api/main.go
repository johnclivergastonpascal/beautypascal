package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Precio struct {
	Cantidad string `json:"cantidad"`
	Valor    string `json:"valor"`
}

type Color struct {
	Nombre string `json:"nombre"`
	Imagen string `json:"imagen,omitempty"`
}

type Producto struct {
	URL             string   `json:"url"`
	Categoria       string   `json:"categoria"`
	Subcategoria    string   `json:"subcategoria"`
	Ubicacion       string   `json:"ubicacion"`
	Titulo          string   `json:"titulo"`
	ImagenesGrandes []string `json:"imagenes"`
	Colores         []Color  `json:"colores"`
	Tamaños         []string `json:"tamaños"`
	DescripcionURL  string   `json:"descripcion_url"`
	Precios         []Precio `json:"precios"`
	BloqueLogistico string   `json:"bloque_logistico"`
}

var productos []Producto

// ------------------ Cargar JSON ------------------
func loadJSON() {
	file, err := os.Open("productos.json")
	if err != nil {
		log.Fatalf("❌ Error abriendo JSON: %v", err)
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatalf("❌ Error leyendo JSON: %v", err)
	}

	err = json.Unmarshal(bytes, &productos)
	if err != nil {
		log.Fatalf("❌ Error parseando JSON: %v", err)
	}

	fmt.Printf("✅ %d productos cargados correctamente\n", len(productos))
}

// ------------------ Utilidades ------------------
func toLower(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func parsePrecio(valor string) float64 {
	valor = strings.ReplaceAll(valor, "$", "")
	valor = strings.ReplaceAll(valor, ",", "")
	p, _ := strconv.ParseFloat(valor, 64)
	return p
}

func getPrecioMinimo(p Producto) float64 {
	min := 999999.0
	for _, pr := range p.Precios {
		v := parsePrecio(pr.Valor)
		if v > 0 && v < min {
			min = v
		}
	}
	if min == 999999.0 {
		return 0
	}
	return min
}

// ------------------ Endpoints ------------------

// 🔹 Todos los productos (paginados y aleatorios)
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

	rand.Seed(time.Now().UnixNano())
	shuffled := make([]Producto, len(productos))
	copy(shuffled, productos)
	rand.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	start := (page - 1) * limit
	end := start + limit
	if start >= len(shuffled) {
		start = len(shuffled)
	}
	if end > len(shuffled) {
		end = len(shuffled)
	}

	response := map[string]interface{}{
		"page":      page,
		"limit":     limit,
		"total":     len(productos),
		"productos": shuffled[start:end],
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// 🔹 Buscar productos
func searchProductos(w http.ResponseWriter, r *http.Request) {
	query := toLower(r.URL.Query().Get("q"))
	categoria := toLower(r.URL.Query().Get("categoria"))
	subcategoria := toLower(r.URL.Query().Get("subcategoria"))
	ubicacion := toLower(r.URL.Query().Get("ubicacion"))
	minStr := r.URL.Query().Get("min")
	maxStr := r.URL.Query().Get("max")

	var minPrecio, maxPrecio float64
	if minStr != "" {
		minPrecio, _ = strconv.ParseFloat(minStr, 64)
	}
	if maxStr != "" {
		maxPrecio, _ = strconv.ParseFloat(maxStr, 64)
	}

	var resultados []Producto
	for _, p := range productos {
		precioMin := getPrecioMinimo(p)

		matchTexto := query == "" ||
			strings.Contains(toLower(p.Titulo), query) ||
			strings.Contains(toLower(p.Categoria), query) ||
			strings.Contains(toLower(p.Subcategoria), query)

		matchCat := categoria == "" || toLower(p.Categoria) == categoria
		matchSub := subcategoria == "" || toLower(p.Subcategoria) == subcategoria
		matchUbic := ubicacion == "" || toLower(p.Ubicacion) == ubicacion

		matchPrecio := (minPrecio == 0 || precioMin >= minPrecio) &&
			(maxPrecio == 0 || precioMin <= maxPrecio)

		if matchTexto && matchCat && matchSub && matchUbic && matchPrecio {
			resultados = append(resultados, p)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resultados)
}

// 🔹 Categorías (todas o una específica)
func getCategorias(w http.ResponseWriter, r *http.Request) {
	param := toLower(r.URL.Query().Get("categoria"))
	categorias := make(map[string]map[string]bool)

	for _, p := range productos {
		cat := strings.TrimSpace(p.Categoria)
		sub := strings.TrimSpace(p.Subcategoria)
		if cat == "" {
			continue
		}
		if categorias[cat] == nil {
			categorias[cat] = make(map[string]bool)
		}
		if sub != "" {
			categorias[cat][sub] = true
		}
	}

	w.Header().Set("Content-Type", "application/json")

	// 🔸 Si no se pasa ningún parámetro, devolver todas las categorías
	if param == "" {
		var lista []map[string]interface{}
		for cat, subs := range categorias {
			var subList []string
			for sub := range subs {
				subList = append(subList, sub)
			}
			sort.Strings(subList)
			lista = append(lista, map[string]interface{}{
				"categoria":     cat,
				"subcategorias": subList,
			})
		}
		sort.Slice(lista, func(i, j int) bool {
			return lista[i]["categoria"].(string) < lista[j]["categoria"].(string)
		})
		json.NewEncoder(w).Encode(lista)
		return
	}

	// 🔸 Si se pasa ?categoria=nombre, devolver solo esa
	for cat, subs := range categorias {
		if toLower(cat) == param {
			var subList []string
			for sub := range subs {
				subList = append(subList, sub)
			}
			sort.Strings(subList)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"categoria":     cat,
				"subcategorias": subList,
			})
			return
		}
	}

	// 🔸 Si no se encuentra la categoría
	w.WriteHeader(http.StatusNotFound)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "Categoría no encontrada",
	})
}

// ------------------ MAIN ------------------
func main() {
	loadJSON()

	http.HandleFunc("/productos", getAllProductos)
	http.HandleFunc("/buscar", searchProductos)
	http.HandleFunc("/categorias", getCategorias)

	fmt.Println("🚀 Servidor corriendo en http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
