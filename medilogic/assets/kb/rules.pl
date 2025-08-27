% ======== rules.pl (NO lo sobreescribe el Admin) ========

% --- Directivas (Ichiban requiere paréntesis) ---
:- dynamic(presente/2).
:- dynamic(alergia/1).
:- dynamic(cronica/1).
:- dynamic(enf_contra_medicamento/2).

% Hechos estáticos vienen del .pl de Admin:
%   sintoma(S).
%   enfermedad(Id, "Nombre", Sistema, Tipo).
%   enf_sintoma(Enf, Sintoma).
%   medicamento(Med).
%   trata(Med, Enf).
%   contraindicado(Med, Cond).
%   enf_contra_medicamento(Enf, Med).   % opcional

peso_max_por_sintoma(3).

% -------- utilidades listas/sets (sin depender de lib externa) --------
member(X, [X|_]).
member(X, [_|T]) :- member(X, T).

% -------- DEDUP de síntomas requeridos por enfermedad --------
reqs_enf(Enf, Reqs) :-
    ( setof(S, enf_sintoma(Enf, S), S0) -> true ; S0 = [] ),
    sort(S0, Reqs).

max_puntaje_enf(Enf, Max) :-
    reqs_enf(Enf, Reqs),
    length(Reqs, N),
    peso_max_por_sintoma(PM),
    Max is N * PM.

puntaje_enf(Enf, Puntaje, Matched) :-
    reqs_enf(Enf, Reqs),
    findall((S,P), (presente(S,P), member(S, Reqs)), Pairs0),
    sort(Pairs0, Pairs),   % dedup por si se repitió el mismo síntoma
    sum_pairs(Pairs, Puntaje),
    Matched = Pairs.

sum_pairs([], 0).
sum_pairs([(_,P)|T], S) :- sum_pairs(T, S1), S is S1 + P.

afinidad(Enf, Afinidad, Matched) :-
    max_puntaje_enf(Enf, Max),
    ( Max =:= 0 -> Afinidad = 0, Matched = []
    ; puntaje_enf(Enf, Puntaje, Matched),
      Afinidad is round(Puntaje * 100 / Max)
    ).

% -------- Urgencia simple --------
urgencia(U) :-
    ( presente(disnea, P), P >= 2 -> U = 'Consulta médica inmediata sugerida'
    ; presente(dolor_pecho, _)    -> U = 'Consulta médica inmediata sugerida'
    ; presente(fiebre, P), P >= 2 -> U = 'Observación recomendada'
    ; U = 'Posible automanejo'
    ).

% -------- Bloqueos explícitos (más claro p/negación) --------
bloqueado_por_alergia(Med) :- alergia(Cond),  contraindicado(Med, Cond).
bloqueado_por_cronica(Med) :- cronica(Cond),  contraindicado(Med, Cond).
bloqueado_por_enf(Enf, Med) :- enf_contra_medicamento(Enf, Med).

% -------- Medicamento seguro: trata y no está bloqueado --------
medicamento_seguro(Enf, Med) :-
    trata(Med, Enf),
    \+ bloqueado_por_alergia(Med),
    \+ bloqueado_por_cronica(Med),
    \+ bloqueado_por_enf(Enf, Med).

% (opcional) detalle para depurar
detalle_enf(Enf, Reqs, Matched, Puntaje, Max, Afinidad) :-
    reqs_enf(Enf, Reqs),
    puntaje_enf(Enf, Puntaje, Matched),
    max_puntaje_enf(Enf, Max),
    afinidad(Enf, Afinidad, _).
