#include <Python.h>
#include <string>
#include <vector>
#include <memory>
#include <iostream>
#include <cstring>
#include <mutex>
#include <map>
#include <thread>
#include "spacy_wrapper.h"

// Global state for Python
static bool g_python_initialized = false;
static std::mutex g_init_mutex;
static PyObject* g_spacy_module = nullptr;
static std::map<std::string, PyObject*> g_nlp_models;
static std::mutex g_models_mutex;
static PyThreadState* g_main_thread_state = nullptr;

class ScopedGIL {
public:
    ScopedGIL() {
        gstate = PyGILState_Ensure();
    }
    ~ScopedGIL() {
        PyGILState_Release(gstate);
    }
private:
    PyGILState_STATE gstate;
};

// Internal structures
struct Chunk {
    std::string text;
    std::string root_text;
    std::string root_dep;
    int start;
    int end;
};

struct VectorData {
    std::vector<double> vector;
    size_t size;
    bool has_vector;
};

struct MorphFeature {
    std::string key;
    std::string value;
};

class SpacyWrapper {
private:
    PyObject* nlp;
    std::string model_name;

public:
    SpacyWrapper(const char* model) : nlp(nullptr), model_name(model) {
        ScopedGIL gil;

        // Check if model is already loaded
        {
            std::lock_guard<std::mutex> lock(g_models_mutex);
            auto it = g_nlp_models.find(model_name);
            if (it != g_nlp_models.end()) {
                nlp = it->second;
                Py_INCREF(nlp);
                return;
            }
        }

        // Load the model
        PyObject* loadFunc = PyObject_GetAttrString(g_spacy_module, "load");
        if (!loadFunc) {
            PyErr_Print();
            throw std::runtime_error("Failed to get spacy.load function");
        }

        PyObject* args = PyTuple_Pack(1, PyUnicode_FromString(model));
        nlp = PyObject_CallObject(loadFunc, args);
        Py_DECREF(args);
        Py_DECREF(loadFunc);

        if (!nlp) {
            PyErr_Print();
            throw std::runtime_error("Failed to load spacy model");
        }

        // Cache the model
        {
            std::lock_guard<std::mutex> lock(g_models_mutex);
            g_nlp_models[model_name] = nlp;
            Py_INCREF(nlp); // Keep a reference for the cache
        }
    }

    ~SpacyWrapper() {
        if (nlp) {
            ScopedGIL gil;
            Py_DECREF(nlp);
        }
    }

    std::vector<Token> tokenize(const std::string& text) {
        ScopedGIL gil;
        std::vector<Token> tokens;

        PyObject* pyText = PyUnicode_FromString(text.c_str());
        PyObject* doc = PyObject_CallFunctionObjArgs(nlp, pyText, NULL);

        if (!doc) {
            PyErr_Print();
            Py_DECREF(pyText);
            return tokens;
        }

        PyObject* iterator = PyObject_GetIter(doc);
        PyObject* token;

        while ((token = PyIter_Next(iterator))) {
            Token t;

            PyObject* textAttr = PyObject_GetAttrString(token, "text");
            if (textAttr) {
                const char* val = PyUnicode_AsUTF8(textAttr);
                if (val) t.text = val;
                Py_DECREF(textAttr);
            }

            PyObject* lemmaAttr = PyObject_GetAttrString(token, "lemma_");
            if (lemmaAttr) {
                const char* val = PyUnicode_AsUTF8(lemmaAttr);
                if (val) t.lemma = val;
                Py_DECREF(lemmaAttr);
            }

            PyObject* posAttr = PyObject_GetAttrString(token, "pos_");
            if (posAttr) {
                const char* val = PyUnicode_AsUTF8(posAttr);
                if (val) t.pos = val;
                Py_DECREF(posAttr);
            }

            PyObject* tagAttr = PyObject_GetAttrString(token, "tag_");
            if (tagAttr) {
                const char* val = PyUnicode_AsUTF8(tagAttr);
                if (val) t.tag = val;
                Py_DECREF(tagAttr);
            }

            PyObject* depAttr = PyObject_GetAttrString(token, "dep_");
            if (depAttr) {
                const char* val = PyUnicode_AsUTF8(depAttr);
                if (val) t.dep = val;
                Py_DECREF(depAttr);
            }

            PyObject* isStopAttr = PyObject_GetAttrString(token, "is_stop");
            if (isStopAttr) {
                t.is_stop = PyObject_IsTrue(isStopAttr);
                Py_DECREF(isStopAttr);
            }

            PyObject* isPunctAttr = PyObject_GetAttrString(token, "is_punct");
            if (isPunctAttr) {
                t.is_punct = PyObject_IsTrue(isPunctAttr);
                Py_DECREF(isPunctAttr);
            }

            PyObject* idxAttr = PyObject_GetAttrString(token, "idx");
            if (idxAttr) {
                t.start = PyLong_AsLong(idxAttr);
                Py_DECREF(idxAttr);
            }

            PyObject* endAttr = PyLong_FromLong(t.start + (t.text.length()));
            if (endAttr) {
                t.end = PyLong_AsLong(endAttr);
                Py_DECREF(endAttr);
            }

            tokens.push_back(t);
            Py_DECREF(token);
        }

        Py_DECREF(iterator);
        Py_DECREF(doc);
        Py_DECREF(pyText);

        return tokens;
    }

    std::vector<Entity> extractEntities(const std::string& text) {
        ScopedGIL gil;
        std::vector<Entity> entities;

        PyObject* pyText = PyUnicode_FromString(text.c_str());
        PyObject* doc = PyObject_CallFunctionObjArgs(nlp, pyText, NULL);

        if (!doc) {
            PyErr_Print();
            Py_DECREF(pyText);
            return entities;
        }

        PyObject* ents = PyObject_GetAttrString(doc, "ents");
        if (!ents) {
            Py_DECREF(doc);
            Py_DECREF(pyText);
            return entities;
        }

        PyObject* iterator = PyObject_GetIter(ents);
        PyObject* ent;

        while ((ent = PyIter_Next(iterator))) {
            Entity e;

            PyObject* textAttr = PyObject_GetAttrString(ent, "text");
            if (textAttr) {
                const char* val = PyUnicode_AsUTF8(textAttr);
                if (val) e.text = val;
                Py_DECREF(textAttr);
            }

            PyObject* labelAttr = PyObject_GetAttrString(ent, "label_");
            if (labelAttr) {
                const char* val = PyUnicode_AsUTF8(labelAttr);
                if (val) e.label = val;
                Py_DECREF(labelAttr);
            }

            PyObject* startAttr = PyObject_GetAttrString(ent, "start_char");
            if (startAttr) {
                e.start = PyLong_AsLong(startAttr);
                Py_DECREF(startAttr);
            }

            PyObject* endAttr = PyObject_GetAttrString(ent, "end_char");
            if (endAttr) {
                e.end = PyLong_AsLong(endAttr);
                Py_DECREF(endAttr);
            }

            entities.push_back(e);
            Py_DECREF(ent);
        }

        Py_DECREF(iterator);
        Py_DECREF(ents);
        Py_DECREF(doc);
        Py_DECREF(pyText);

        return entities;
    }

    std::vector<std::string> splitSentences(const std::string& text) {
        ScopedGIL gil;
        std::vector<std::string> sentences;

        PyObject* pyText = PyUnicode_FromString(text.c_str());
        PyObject* doc = PyObject_CallFunctionObjArgs(nlp, pyText, NULL);

        if (!doc) {
            PyErr_Print();
            Py_DECREF(pyText);
            return sentences;
        }

        PyObject* sents = PyObject_GetAttrString(doc, "sents");
        if (!sents) {
            Py_DECREF(doc);
            Py_DECREF(pyText);
            return sentences;
        }

        PyObject* iterator = PyObject_GetIter(sents);
        PyObject* sent;

        while ((sent = PyIter_Next(iterator))) {
            PyObject* textAttr = PyObject_GetAttrString(sent, "text");
            if (textAttr) {
                const char* val = PyUnicode_AsUTF8(textAttr);
                if (val) sentences.push_back(val);
                Py_DECREF(textAttr);
            }
            Py_DECREF(sent);
        }

        Py_DECREF(iterator);
        Py_DECREF(sents);
        Py_DECREF(doc);
        Py_DECREF(pyText);

        return sentences;
    }

    std::vector<Chunk> getNounChunks(const std::string& text) {
        ScopedGIL gil;
        std::vector<Chunk> chunks;

        PyObject* pyText = PyUnicode_FromString(text.c_str());
        PyObject* doc = PyObject_CallFunctionObjArgs(nlp, pyText, NULL);

        if (!doc) {
            PyErr_Print();
            Py_DECREF(pyText);
            return chunks;
        }

        PyObject* noun_chunks = PyObject_GetAttrString(doc, "noun_chunks");
        if (!noun_chunks) {
            Py_DECREF(doc);
            Py_DECREF(pyText);
            return chunks;
        }

        PyObject* iterator = PyObject_GetIter(noun_chunks);
        PyObject* chunk;

        while ((chunk = PyIter_Next(iterator))) {
            Chunk c;

            PyObject* textAttr = PyObject_GetAttrString(chunk, "text");
            if (textAttr) {
                const char* val = PyUnicode_AsUTF8(textAttr);
                if (val) c.text = val;
                Py_DECREF(textAttr);
            }

            PyObject* rootAttr = PyObject_GetAttrString(chunk, "root");
            if (rootAttr) {
                PyObject* rootText = PyObject_GetAttrString(rootAttr, "text");
                if (rootText) {
                    const char* val = PyUnicode_AsUTF8(rootText);
                    if (val) c.root_text = val;
                    Py_DECREF(rootText);
                }
                PyObject* rootDep = PyObject_GetAttrString(rootAttr, "dep_");
                if (rootDep) {
                    const char* val = PyUnicode_AsUTF8(rootDep);
                    if (val) c.root_dep = val;
                    Py_DECREF(rootDep);
                }
                Py_DECREF(rootAttr);
            }

            PyObject* startAttr = PyObject_GetAttrString(chunk, "start_char");
            if (startAttr) {
                c.start = PyLong_AsLong(startAttr);
                Py_DECREF(startAttr);
            }

            PyObject* endAttr = PyObject_GetAttrString(chunk, "end_char");
            if (endAttr) {
                c.end = PyLong_AsLong(endAttr);
                Py_DECREF(endAttr);
            }

            chunks.push_back(c);
            Py_DECREF(chunk);
        }

        Py_DECREF(iterator);
        Py_DECREF(noun_chunks);
        Py_DECREF(doc);
        Py_DECREF(pyText);

        return chunks;
    }

    VectorData getVector(const std::string& text) {
        ScopedGIL gil;
        VectorData vecData;
        vecData.vector.clear();
        vecData.size = 0;
        vecData.has_vector = false;

        PyObject* pyText = PyUnicode_FromString(text.c_str());
        PyObject* doc = PyObject_CallFunctionObjArgs(nlp, pyText, NULL);

        if (!doc) {
            PyErr_Print();
            Py_DECREF(pyText);
            return vecData;
        }

        PyObject* hasVecAttr = PyObject_GetAttrString(doc, "has_vector");
        if (hasVecAttr) {
            vecData.has_vector = PyObject_IsTrue(hasVecAttr);
            Py_DECREF(hasVecAttr);
        }

        if (vecData.has_vector) {
            PyObject* vectorAttr = PyObject_GetAttrString(doc, "vector");
            if (vectorAttr) {
                // Convert numpy array to C array
                PyObject* listObj = PyObject_CallMethod(vectorAttr, "tolist", NULL);
                if (listObj && PyList_Check(listObj)) {
                    Py_ssize_t size = PyList_Size(listObj);
                    vecData.size = size;
                    vecData.vector.resize(size);

                    for (Py_ssize_t i = 0; i < size; i++) {
                        PyObject* item = PyList_GetItem(listObj, i);
                        vecData.vector[i] = PyFloat_AsDouble(item);
                    }
                    Py_DECREF(listObj);
                }
                Py_DECREF(vectorAttr);
            }
        }

        Py_DECREF(doc);
        Py_DECREF(pyText);

        return vecData;
    }

    double getSimilarity(const std::string& text1, const std::string& text2) {
        ScopedGIL gil;

        PyObject* pyText1 = PyUnicode_FromString(text1.c_str());
        PyObject* doc1 = PyObject_CallFunctionObjArgs(nlp, pyText1, NULL);

        if (!doc1) {
            PyErr_Print();
            Py_DECREF(pyText1);
            return 0.0;
        }

        PyObject* pyText2 = PyUnicode_FromString(text2.c_str());
        PyObject* doc2 = PyObject_CallFunctionObjArgs(nlp, pyText2, NULL);

        if (!doc2) {
            PyErr_Print();
            Py_DECREF(doc1);
            Py_DECREF(pyText1);
            Py_DECREF(pyText2);
            return 0.0;
        }

        double similarity = 0.0;
        PyObject* simMethod = PyObject_GetAttrString(doc1, "similarity");
        if (simMethod) {
            PyObject* args = PyTuple_Pack(1, doc2);
            PyObject* result = PyObject_CallObject(simMethod, args);
            if (result) {
                similarity = PyFloat_AsDouble(result);
                Py_DECREF(result);
            }
            Py_DECREF(args);
            Py_DECREF(simMethod);
        }

        Py_DECREF(doc2);
        Py_DECREF(pyText2);
        Py_DECREF(doc1);
        Py_DECREF(pyText1);

        return similarity;
    }

    std::vector<MorphFeature> getMorphology(const std::string& text) {
        ScopedGIL gil;
        std::vector<MorphFeature> morphFeatures;

        PyObject* pyText = PyUnicode_FromString(text.c_str());
        PyObject* doc = PyObject_CallFunctionObjArgs(nlp, pyText, NULL);

        if (!doc) {
            PyErr_Print();
            Py_DECREF(pyText);
            return morphFeatures;
        }

        PyObject* iterator = PyObject_GetIter(doc);
        PyObject* token;

        while ((token = PyIter_Next(iterator))) {
            PyObject* morphAttr = PyObject_GetAttrString(token, "morph");
            if (morphAttr) {
                PyObject* strMethod = PyObject_GetAttrString(morphAttr, "__str__");
                if (strMethod) {
                    PyObject* morphStr = PyObject_CallObject(strMethod, NULL);
                    if (morphStr) {
                        const char* morphText = PyUnicode_AsUTF8(morphStr);
                        if (!morphText) {
                            Py_DECREF(morphStr);
                            Py_DECREF(strMethod);
                            Py_DECREF(morphAttr);
                            Py_DECREF(token);
                            continue;
                        }
                        // Parse morph string (e.g., "Case=Nom|Number=Sing")
                        std::string morphString(morphText);
                        if (!morphString.empty()) {
                            // For simplicity, store the entire morph string as one feature
                            MorphFeature feature;
                            feature.key = "morph";
                            feature.value = morphString;
                            morphFeatures.push_back(feature);
                        }
                        Py_DECREF(morphStr);
                    }
                    Py_DECREF(strMethod);
                }
                Py_DECREF(morphAttr);
            }
            Py_DECREF(token);
        }

        Py_DECREF(iterator);
        Py_DECREF(doc);
        Py_DECREF(pyText);

        return morphFeatures;
    }
};

// Global wrapper instance with mutex protection
static std::unique_ptr<SpacyWrapper> g_wrapper;
static std::mutex g_wrapper_mutex;

extern "C" {

int spacy_init(const char* model_name) {
    if (!model_name || strlen(model_name) == 0) {
        std::cerr << "Error: model_name is null or empty" << std::endl;
        return -1;
    }

    // Initialize Python if needed
    {
        std::lock_guard<std::mutex> lock(g_init_mutex);
        if (!g_python_initialized) {
            Py_Initialize();

            // Save the main thread state and release GIL
            // PyEval_InitThreads() is automatic in Python 3.7+
            g_main_thread_state = PyEval_SaveThread();
            g_python_initialized = true;
        }
    }

    try {
        // Ensure GIL is held for this thread
        ScopedGIL gil;

        // Import spacy if not already done
        if (!g_spacy_module) {
            // Setup Python path
            PyObject* sys = PyImport_ImportModule("sys");
            if (sys) {
                PyObject* path = PyObject_GetAttrString(sys, "path");
                if (path) {
                    PyList_Append(path, PyUnicode_FromString("."));
                    PyList_Append(path, PyUnicode_FromString("./.venv/lib/python3.12/site-packages"));
                    Py_DECREF(path);
                }
                Py_DECREF(sys);
            }

            g_spacy_module = PyImport_ImportModule("spacy");
            if (!g_spacy_module) {
                PyErr_Print();
                return -1;
            }
        }

        // Create global wrapper instance
        {
            std::lock_guard<std::mutex> lock(g_wrapper_mutex);
            if (!g_wrapper) {
                g_wrapper = std::make_unique<SpacyWrapper>(model_name);
            }
        }
        return 0;
    } catch (const std::exception& e) {
        std::cerr << "Error initializing Spacy: " << e.what() << std::endl;
        return -1;
    }
}

void spacy_cleanup() {
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);
    g_wrapper.reset();
}

TokenArray spacy_tokenize(const char* text) {
    TokenArray result = {nullptr, 0};

    if (!text) {
        return result;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return result;
    }

    try {
        auto tokens = g_wrapper->tokenize(text);
        if (tokens.empty()) {
            return result;
        }

        result.tokens = (CToken*)malloc(sizeof(CToken) * tokens.size());
        if (!result.tokens) {
            std::cerr << "Error: Failed to allocate memory for tokens" << std::endl;
            return result;
        }

        result.count = tokens.size();

        for (size_t i = 0; i < tokens.size(); ++i) {
            result.tokens[i].text = strdup(tokens[i].text.c_str());
            result.tokens[i].lemma = strdup(tokens[i].lemma.c_str());
            result.tokens[i].pos = strdup(tokens[i].pos.c_str());
            result.tokens[i].tag = strdup(tokens[i].tag.c_str());
            result.tokens[i].dep = strdup(tokens[i].dep.c_str());
            result.tokens[i].is_stop = tokens[i].is_stop;
            result.tokens[i].is_punct = tokens[i].is_punct;
            result.tokens[i].start = tokens[i].start;
            result.tokens[i].end = tokens[i].end;
        }
    } catch (const std::exception& e) {
        std::cerr << "Error in tokenize: " << e.what() << std::endl;
        if (result.tokens) {
            free(result.tokens);
            result.tokens = nullptr;
            result.count = 0;
        }
    }

    return result;
}

void free_token_array(TokenArray* arr) {
    if (arr && arr->tokens) {
        for (size_t i = 0; i < arr->count; ++i) {
            free((void*)arr->tokens[i].text);
            free((void*)arr->tokens[i].lemma);
            free((void*)arr->tokens[i].pos);
            free((void*)arr->tokens[i].tag);
            free((void*)arr->tokens[i].dep);
        }
        free(arr->tokens);
        arr->tokens = nullptr;
        arr->count = 0;
    }
}

EntityArray spacy_extract_entities(const char* text) {
    EntityArray result = {nullptr, 0};

    if (!text) {
        return result;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return result;
    }

    try {
        auto entities = g_wrapper->extractEntities(text);
        if (entities.empty()) {
            return result;
        }

        result.entities = (CEntity*)malloc(sizeof(CEntity) * entities.size());
        if (!result.entities) {
            std::cerr << "Error: Failed to allocate memory for entities" << std::endl;
            return result;
        }

        result.count = entities.size();

        for (size_t i = 0; i < entities.size(); ++i) {
            result.entities[i].text = strdup(entities[i].text.c_str());
            result.entities[i].label = strdup(entities[i].label.c_str());
            result.entities[i].start = entities[i].start;
            result.entities[i].end = entities[i].end;
        }
    } catch (const std::exception& e) {
        std::cerr << "Error in extract_entities: " << e.what() << std::endl;
        if (result.entities) {
            free(result.entities);
            result.entities = nullptr;
            result.count = 0;
        }
    }

    return result;
}

void free_entity_array(EntityArray* arr) {
    if (arr && arr->entities) {
        for (size_t i = 0; i < arr->count; ++i) {
            free((void*)arr->entities[i].text);
            free((void*)arr->entities[i].label);
        }
        free(arr->entities);
        arr->entities = nullptr;
        arr->count = 0;
    }
}

SentenceArray spacy_split_sentences(const char* text) {
    SentenceArray result = {nullptr, 0};

    if (!text) {
        return result;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return result;
    }

    try {
        auto sentences = g_wrapper->splitSentences(text);
        if (sentences.empty()) {
            return result;
        }

        result.sentences = (char**)malloc(sizeof(char*) * sentences.size());
        if (!result.sentences) {
            std::cerr << "Error: Failed to allocate memory for sentences" << std::endl;
            return result;
        }

        result.count = sentences.size();

        for (size_t i = 0; i < sentences.size(); ++i) {
            result.sentences[i] = strdup(sentences[i].c_str());
        }
    } catch (const std::exception& e) {
        std::cerr << "Error in split_sentences: " << e.what() << std::endl;
        if (result.sentences) {
            free(result.sentences);
            result.sentences = nullptr;
            result.count = 0;
        }
    }

    return result;
}

void free_sentence_array(SentenceArray* arr) {
    if (arr && arr->sentences) {
        for (size_t i = 0; i < arr->count; ++i) {
            free(arr->sentences[i]);
        }
        free(arr->sentences);
        arr->sentences = nullptr;
        arr->count = 0;
    }
}

ChunkArray spacy_get_noun_chunks(const char* text) {
    ChunkArray result = {nullptr, 0};

    if (!text) {
        return result;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return result;
    }

    try {
        auto chunks = g_wrapper->getNounChunks(text);
        if (chunks.empty()) {
            return result;
        }

        result.chunks = (CChunk*)malloc(sizeof(CChunk) * chunks.size());
        if (!result.chunks) {
            std::cerr << "Error: Failed to allocate memory for chunks" << std::endl;
            return result;
        }

        result.count = chunks.size();

        for (size_t i = 0; i < chunks.size(); ++i) {
            result.chunks[i].text = strdup(chunks[i].text.c_str());
            result.chunks[i].root_text = strdup(chunks[i].root_text.c_str());
            result.chunks[i].root_dep = strdup(chunks[i].root_dep.c_str());
            result.chunks[i].start = chunks[i].start;
            result.chunks[i].end = chunks[i].end;
        }
    } catch (const std::exception& e) {
        std::cerr << "Error in get_noun_chunks: " << e.what() << std::endl;
        if (result.chunks) {
            free(result.chunks);
            result.chunks = nullptr;
            result.count = 0;
        }
    }

    return result;
}

void free_chunk_array(ChunkArray* arr) {
    if (arr && arr->chunks) {
        for (size_t i = 0; i < arr->count; ++i) {
            free((void*)arr->chunks[i].text);
            free((void*)arr->chunks[i].root_text);
            free((void*)arr->chunks[i].root_dep);
        }
        free(arr->chunks);
        arr->chunks = nullptr;
        arr->count = 0;
    }
}

CVectorData spacy_get_vector(const char* text) {
    CVectorData result = {nullptr, 0, false};

    if (!text) {
        return result;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return result;
    }

    try {
        auto vecData = g_wrapper->getVector(text);
        result.has_vector = vecData.has_vector;
        result.size = vecData.size;

        if (vecData.has_vector && vecData.size > 0) {
            result.vector = (double*)malloc(sizeof(double) * vecData.size);
            if (result.vector) {
                for (size_t i = 0; i < vecData.size; ++i) {
                    result.vector[i] = vecData.vector[i];
                }
            }
        }
    } catch (const std::exception& e) {
        std::cerr << "Error in get_vector: " << e.what() << std::endl;
        if (result.vector) {
            free(result.vector);
            result.vector = nullptr;
            result.size = 0;
            result.has_vector = false;
        }
    }

    return result;
}

void free_vector_data(CVectorData* vec) {
    if (vec && vec->vector) {
        free(vec->vector);
        vec->vector = nullptr;
        vec->size = 0;
        vec->has_vector = false;
    }
}

double spacy_similarity(const char* text1, const char* text2) {
    if (!text1 || !text2) {
        return 0.0;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return 0.0;
    }

    try {
        return g_wrapper->getSimilarity(text1, text2);
    } catch (const std::exception& e) {
        std::cerr << "Error in similarity: " << e.what() << std::endl;
        return 0.0;
    }
}

MorphArray spacy_get_morphology(const char* text) {
    MorphArray result = {nullptr, 0};

    if (!text) {
        return result;
    }

    // Lock wrapper access
    std::lock_guard<std::mutex> lock(g_wrapper_mutex);

    if (!g_wrapper) {
        std::cerr << "Error: Spacy not initialized" << std::endl;
        return result;
    }

    try {
        auto morphFeatures = g_wrapper->getMorphology(text);
        if (morphFeatures.empty()) {
            return result;
        }

        result.features = (CMorphFeature*)malloc(sizeof(CMorphFeature) * morphFeatures.size());
        if (!result.features) {
            std::cerr << "Error: Failed to allocate memory for morph features" << std::endl;
            return result;
        }

        result.count = morphFeatures.size();

        for (size_t i = 0; i < morphFeatures.size(); ++i) {
            result.features[i].morph_key = strdup(morphFeatures[i].key.c_str());
            result.features[i].morph_value = strdup(morphFeatures[i].value.c_str());
        }
    } catch (const std::exception& e) {
        std::cerr << "Error in get_morphology: " << e.what() << std::endl;
        if (result.features) {
            free(result.features);
            result.features = nullptr;
            result.count = 0;
        }
    }

    return result;
}

void free_morph_array(MorphArray* arr) {
    if (arr && arr->features) {
        for (size_t i = 0; i < arr->count; ++i) {
            free((void*)arr->features[i].morph_key);
            free((void*)arr->features[i].morph_value);
        }
        free(arr->features);
        arr->features = nullptr;
        arr->count = 0;
    }
}

}